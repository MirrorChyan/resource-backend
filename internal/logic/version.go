package logic

import (
	"bytes"
	"context"
	"errors"

	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	. "github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/logic/dispense"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/oss"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/MirrorChyan/resource-backend/internal/tasks"
	"github.com/MirrorChyan/resource-backend/internal/vercomp"
	"github.com/bytedance/sonic"
	"github.com/go-redsync/redsync/v4"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
)

type VersionLogic struct {
	logger          *zap.Logger
	repo            *repo.Repo
	rawQuery        *repo.RawQuery
	versionRepo     *repo.Version
	distributeLogic *dispense.DistributeLogic
	storageLogic    *StorageLogic
	resourceLogic   *ResourceLogic
	comparator      *vercomp.VersionComparator
	taskQueue       *tasks.TaskQueue
	rdb             *redis.Client
	sync            *redsync.Redsync
	cacheGroup      *cache.MultiCacheGroup
}

func NewVersionLogic(
	logger *zap.Logger,
	repo *repo.Repo,
	versionRepo *repo.Version,
	rawQuery *repo.RawQuery,
	verComparator *vercomp.VersionComparator,
	distributeLogic *dispense.DistributeLogic,
	resourceLogic *ResourceLogic,
	storageLogic *StorageLogic,
	rdb *redis.Client,
	sync *redsync.Redsync,
	taskQueue *tasks.TaskQueue,
	cacheGroup *cache.MultiCacheGroup,
) *VersionLogic {
	l := &VersionLogic{
		logger:          logger,
		repo:            repo,
		versionRepo:     versionRepo,
		storageLogic:    storageLogic,
		resourceLogic:   resourceLogic,
		distributeLogic: distributeLogic,
		comparator:      verComparator,
		rawQuery:        rawQuery,
		taskQueue:       taskQueue,
		rdb:             rdb,
		sync:            sync,
		cacheGroup:      cacheGroup,
	}
	InitAsynqServer(logger, l)
	return l
}

func (l *VersionLogic) GetRedisClient() *redis.Client {
	return l.rdb
}

func (l *VersionLogic) GetCacheGroup() *cache.MultiCacheGroup {
	return l.cacheGroup
}

func (l *VersionLogic) GetVersionChannel(channel string) version.Channel {
	switch channel {
	case types.ChannelStable.String():
		return version.ChannelStable
	case types.ChannelBeta.String():
		return version.ChannelBeta
	case types.ChannelAlpha.String():
		return version.ChannelAlpha
	default:
		return version.ChannelStable
	}
}

func (l *VersionLogic) GetVersionByName(ctx context.Context, param GetVersionByNameParam) (*ent.Version, error) {
	return l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.VersionName)

}

func (l *VersionLogic) ExistNameWithOSAndArch(ctx context.Context, param ExistVersionNameWithOSAndArchParam) (bool, error) {
	ver, err := l.versionRepo.GetVersionByName(ctx, param.ResourceId, param.VersionName)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, nil
		}
		l.logger.Error("Failed to check version name exists",
			zap.Error(err),
		)
		return false, err
	}
	return l.storageLogic.CheckStorageExist(ctx, ver.ID, param.OS, param.Arch)
}

func (l *VersionLogic) GetVersionNumber(ctx context.Context, resID string) (uint64, error) {
	maxNumVer, err := l.versionRepo.GetMaxNumberVersion(ctx, resID)
	if err == nil {
		return maxNumVer.Number + 1, nil
	}

	if ent.IsNotFound(err) {
		return 1, nil
	}

	return 0, err
}

func (l *VersionLogic) CreateVersion(ctx context.Context, resourceId, channel, name string) (*ent.Version, error) {

	number, err := l.GetVersionNumber(ctx, resourceId)
	if err != nil {
		l.logger.Error("Failed to get version number",
			zap.String("resource id", resourceId),
			zap.Error(err),
		)
		return nil, err
	}

	verChannel := l.GetVersionChannel(channel)

	return l.versionRepo.CreateVersion(ctx, resourceId, verChannel, name, number)
}

func (l *VersionLogic) CreatePreSignedUrl(ctx context.Context, param CreateVersionParam) (*oss.SignaturePolicyToken, error) {
	var (
		resourceId  = param.ResourceID
		versionName = param.Name
		system      = param.OS
		arch        = param.Arch
		channel     = param.Channel
		filename    = param.Filename
	)
	ver, err := l.LoadStoreNewVersionTx(ctx, resourceId, versionName, channel)
	if err != nil {
		return nil, err
	}
	dest := l.storageLogic.BuildVersionStorageDirPath(resourceId, ver.ID, system, arch)

	ut, err := l.resourceLogic.FindUpdateTypeById(ctx, resourceId)
	if err != nil {
		l.logger.Error("Failed to find resource",
			zap.String("resource id", resourceId),
			zap.Error(err),
		)
		return nil, err
	}

	if ut == types.UpdateIncremental {
		filename = misc.DefaultResourceName
	}

	token, err := oss.AcquirePolicyToken(l.cleanRootStoragePath(dest), filename)
	if err != nil {
		return nil, err
	}
	return token, err
}

// doVerifyRequiredFileType The file must be in zip format
func (l *VersionLogic) doVerifyRequiredFileType(dest string) bool {
	f, err := os.Open(dest)
	if err != nil {
		l.logger.Error("Failed to open file please check file",
			zap.String("file", dest),
			zap.Error(err),
		)
		return false
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	sniff := make([]byte, misc.SniffLen)
	_, _ = f.Read(sniff)
	return strings.HasSuffix(dest, misc.ZipSuffix) && bytes.HasPrefix(sniff, []byte("PK\x03\x04"))
}

func (l *VersionLogic) ProcessCreateVersionCallback(ctx context.Context, param CreateVersionCallBackParam) error {
	var (
		resourceId = param.ResourceID
		// version name is unique in all channels
		versionName = param.Name
		system      = param.OS
		arch        = param.Arch
		channel     = param.Channel
		filename    = param.Filename
		key         = param.Key
	)
	ver, err := l.versionRepo.GetVersionByName(ctx, resourceId, versionName)
	if err != nil {
		return err
	}

	err = l.repo.WithTx(ctx, func(tx *ent.Tx) error {
		var (
			versionId = ver.ID
		)

		source := filepath.Join(l.storageLogic.OSSDir, key)
		_, err = os.Stat(source)
		if err != nil {
			l.logger.Error("Failed to stat archive file pleas check the oss upload",
				zap.String("archive path", source),
				zap.Error(err),
			)
			return err
		}

		exist, err := l.storageLogic.CheckStorageExist(ctx, versionId, system, arch)
		if err != nil {
			l.logger.Error("Failed to check storage exist",
				zap.Error(err),
			)
			return err
		}
		if exist {
			l.logger.Warn("version storage already exists",
				zap.String("resource id", resourceId),
				zap.String("version name", versionName),
				zap.String("resource os", system),
				zap.String("resource arch", arch),
			)
			return nil
		}

		// make sure the storage dir exists
		dest := filepath.Join(l.storageLogic.RootDir, key)
		_ = os.MkdirAll(filepath.Dir(dest), os.ModePerm)

		l.logger.Debug("start CopyFile")

		if err = fileops.CopyFile(source, dest); err != nil {
			l.logger.Error("failed to copy oss to local storage file",
				zap.String("source", source),
				zap.String("destination", dest),
				zap.Error(err),
			)
			return err
		}

		l.logger.Debug("end CopyFile")

		ut, err := l.resourceLogic.FindUpdateTypeById(ctx, resourceId)
		if err != nil {
			l.logger.Error("Failed to find resource",
				zap.String("resource id", resourceId),
				zap.Error(err),
			)
			return err
		}

		var (
			isIncremental = ut == types.UpdateIncremental

			hashes         = make(map[string]string)
			flatPackageDir string
		)

		if isIncremental {
			if !l.doVerifyRequiredFileType(dest) {
				return misc.NotAllowedFileType
			}
			flatPackageDir = l.storageLogic.BuildVersionResourceStorageDirPath(resourceId, versionId, system, arch)
			tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
				return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
					// Code before the actual rollback.

					go func() {
						if e := os.RemoveAll(flatPackageDir); e != nil {
							l.logger.Error("Failed to remove storage directory",
								zap.Error(e),
							)
						}
					}()

					err := next.Rollback(ctx, tx)
					// Code after the transaction was rolled back.

					return err
				})
			})

			l.logger.Debug("start unpack resource",
				zap.String("save dir", flatPackageDir),
			)
			if err = archive.UnpackZip(dest, flatPackageDir); err != nil {
				l.logger.Error("Failed to unpack file",
					zap.String("version name", versionName),
					zap.Error(err),
				)
				return err
			}
			l.logger.Debug("end unpack resource",
				zap.String("save dir", flatPackageDir),
			)

			l.logger.Debug("start calculate total file hash",
				zap.String("dest dir", flatPackageDir),
			)

			hashes, err = filehash.GetAll(flatPackageDir)
			if err != nil {
				l.logger.Error("Failed to get file hashes",
					zap.String("version name", versionName),
					zap.Error(err),
				)
				return err
			}

			l.logger.Debug("end calculate total file hash",
				zap.String("dest dir", flatPackageDir),
			)
		}

		archivePath := l.storageLogic.BuildVersionResourceStoragePath(resourceId, versionId, system, arch, filename)
		l.logger.Debug("start calculate package hash",
			zap.String("package dir", archivePath),
		)

		packageHash, err := filehash.Calculate(archivePath)
		if err != nil {
			l.logger.Error("Failed to calculate full update package hash",
				zap.String("resource id", resourceId),
				zap.String("version name", versionName),
				zap.String("os", system),
				zap.String("arch", arch),
				zap.Error(err),
			)
			return err
		}

		l.logger.Debug("end calculate package hash",
			zap.String("package dir", archivePath),
		)
		_, err = l.storageLogic.CreateFullUpdateStorage(ctx, tx, versionId, system, arch, archivePath, packageHash, flatPackageDir, hashes)
		if err != nil {
			l.logger.Error("Failed to create storage",
				zap.Error(err),
			)
			return err
		}
		return nil
	})

	if err != nil {
		l.logger.Error("Failed to commit transaction",
			zap.Error(err),
		)
		go l.doWebhookNotify(resourceId, versionName, channel, system, arch, false)
		return err
	}

	go l.doWebhookNotify(resourceId, versionName, channel, system, arch, true)
	l.doPostCreateResources(resourceId)

	return nil
}

func (l *VersionLogic) LoadStoreNewVersionTx(ctx context.Context, resourceId, versionName, channel string) (*ent.Version, error) {
	var (
		mutexKey = strings.Join([]string{misc.LoadStoreNewVersionKey, resourceId, versionName, channel}, ":")
	)
	mutex := l.sync.NewMutex(mutexKey, redsync.WithExpiry(10*time.Second))

	if err := mutex.Lock(); err != nil {
		return nil, err
	}

	c, cancel := context.WithCancel(ctx)
	defer cancel()

	go renewMutex(c, mutex)

	defer func() {
		if ok, err := mutex.Unlock(); !ok || err != nil {
			l.logger.Error("Failed to unlock patch mutex")
		}
	}()

	ver, err := l.versionRepo.GetVersionByName(ctx, resourceId, versionName)
	if err == nil {
		return ver, nil
	}

	if !ent.IsNotFound(err) {
		return nil, err
	}

	ver, err = l.CreateVersion(ctx, resourceId, channel, versionName)
	if err != nil {
		l.logger.Error("Failed to create new version",
			zap.String("resource id", resourceId),
			zap.String("channel", channel),
			zap.String("version name", versionName),
			zap.Error(err),
		)
		return nil, err
	}
	return ver, err
}

func (l *VersionLogic) doWebhookNotify(resourceId, versionName, channel, os, arch string, ok bool) {
	var (
		cfg     = GConfig
		webhook = cfg.Extra.CreateNewVersionWebhook
	)

	buf, e := sonic.Marshal(map[string]string{
		"resource_id":  resourceId,
		"version_name": versionName,
		"channel":      channel,
		"os":           os,
		"arch":         arch,
		"ok":           strconv.FormatBool(ok),
	})
	if e != nil {
		l.logger.Warn("Failed to marshal CreateNewVersion callback")
		return
	}
	_, err := http.Post(webhook, "application/json", bytes.NewBuffer(buf))
	if err != nil {
		l.logger.Warn("Failed to send CreateNewVersion callback")
	}
}

func (l *VersionLogic) doPostCreateResources(resourceId string) {
	cg := l.GetCacheGroup()
	for _, system := range misc.TotalOs {
		for _, arch := range misc.TotalArch {
			for _, channel := range misc.TotalChannel {
				key := cg.GetCacheKey(resourceId, system, arch, channel)
				cg.MultiVersionInfoCache.Delete(key)
			}
		}
	}
}

func (l *VersionLogic) GenerateIncrementalPackage(ctx context.Context, resourceId string, target, current int, system, arch string) error {
	targetInfo, currentInfo, err := l.fetchStorageInfoTuple(ctx, target, current, system, arch)
	if err != nil {
		// only versions exist but no storage exist
		if ent.IsNotFound(err) {
			l.logger.Warn("versions exist but no storage exist please check storage",
				zap.Int("target version id", target),
				zap.Int("current version id", current),
				zap.String("os", system),
				zap.String("arch", arch),
			)
			return nil
		}
		l.logger.Error("failed to get storage info",
			zap.Error(err),
		)
		return err
	}

	err = l.doCreateIncrementalUpdatePackage(ctx, PatchTaskExecuteParam{
		ResourceId:           resourceId,
		TargetResourcePath:   targetInfo.ResourcePath,
		TargetVersionId:      target,
		CurrentVersionId:     current,
		TargetStorageHashes:  targetInfo.FileHashes,
		CurrentStorageHashes: currentInfo.FileHashes,
		OS:                   system,
		Arch:                 arch,
	})
	if err != nil {
		return err
	}

	cacheKey := l.cacheGroup.GetCacheKey(
		strconv.Itoa(target),
		strconv.Itoa(current),
		system,
		arch,
	)
	l.cacheGroup.IncrementalUpdateInfoCache.Delete(cacheKey)

	return nil
}

func (l *VersionLogic) doCreateIncrementalUpdatePackage(ctx context.Context, param PatchTaskExecuteParam) error {

	var (
		resourceId  = param.ResourceId
		target      = param.TargetVersionId
		current     = param.CurrentVersionId
		system      = param.OS
		resArch     = param.Arch
		resourceDir = param.TargetResourcePath
	)

	changes, err := patcher.CalculateDiff(param.TargetStorageHashes, param.CurrentStorageHashes)
	if err != nil {
		l.logger.Error("Failed to calculate diff",
			zap.Error(err),
		)
		return err
	}

	patchDir := l.storageLogic.BuildVersionPatchStorageDirPath(resourceId, target, system, resArch)

	patchName, err := patcher.Generate(strconv.Itoa(current), resourceDir, patchDir, changes)
	if err != nil {
		l.logger.Error("Failed to generate patch package",
			zap.Error(err),
		)
		return err
	}

	source := filepath.Join(patchDir, patchName)

	err = l.repo.WithTx(ctx, func(tx *ent.Tx) (err error) {

		tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
			return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
				// Code before the actual rollback.
				if err := os.RemoveAll(source); err != nil {
					l.logger.Error("Failed to remove patch package",
						zap.Error(err),
					)
				}
				err := next.Rollback(ctx, tx)
				// Code after the transaction was rolled back.
				return err
			})
		})

		hashes, err := filehash.Calculate(source)
		if err != nil {
			l.logger.Error("Failed to calculate incremental update package hash",
				zap.String("resource id", resourceId),
				zap.Int("target version id", target),
				zap.Int("current version id", current),
				zap.String("os", system),
				zap.String("arch", resArch),
				zap.Error(err),
			)
			return err
		}
		_, err = l.storageLogic.CreateIncrementalUpdateStorage(ctx, tx, target, current, system, resArch, source, hashes)
		if err != nil {
			l.logger.Error("Failed to create incremental update storage",
				zap.Error(err),
			)
			return err
		}

		return nil
	})

	if err != nil {
		l.logger.Error("Failed to commit transaction",
			zap.Error(err),
		)
		return err
	}

	dest := filepath.Join(l.storageLogic.OSSDir, l.cleanRootStoragePath(source))
	_ = os.MkdirAll(filepath.Dir(dest), os.ModePerm)
	err = fileops.CopyFile(source, dest)
	if err != nil {
		l.logger.Error("failed to copy local storage to oss file",
			zap.String("source", source),
			zap.String("destination", dest),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func (l *VersionLogic) GetMultiLatestVersionInfo(resourceId, os, arch, channel string) (*LatestVersionInfo, error) {
	var (
		key = l.cacheGroup.GetCacheKey(resourceId, os, arch, channel)
	)
	val, err := l.cacheGroup.MultiVersionInfoCache.ComputeIfAbsent(key, func() (*MultiVersionInfo, error) {
		info, err := l.doGetLatestVersionInfo(resourceId, os, arch, channel)
		switch {
		case err == nil:
			return &MultiVersionInfo{LatestVersionInfo: info}, nil
		case errors.Is(err, misc.ResourceNotFound):
			return &MultiVersionInfo{}, nil
		}
		return nil, err
	})
	if err != nil {
		return nil, err
	}

	if info := (*val).LatestVersionInfo; info != nil {
		if !info.PackagePath.Valid {
			l.logger.Error("latest resource version storage not found please check storage path",
				zap.String("resource id", resourceId),
				zap.String("os", os),
				zap.String("arch", arch),
				zap.String("channel", channel),
			)
			return nil, misc.StorageInfoNotFound
		}

		ut, err := l.resourceLogic.FindUpdateTypeById(context.Background(), resourceId)
		if err != nil {
			return nil, err
		}
		info.ResourceUpdateType = ut

		return info, nil
	}
	return nil, misc.ResourceNotFound
}

func (l *VersionLogic) doGetLatestVersionInfo(resourceId, os, arch, channel string) (*LatestVersionInfo, error) {
	info, err := l.rawQuery.GetSpecifiedLatestVersion(resourceId, os, arch)
	if err != nil {
		return nil, err
	}
	if len(info) == 0 {
		return nil, misc.ResourceNotFound
	}

	var stable, beta, alpha *LatestVersionInfo

	for i := range info {
		data := info[i]
		switch data.Channel {
		case types.ChannelStable.String():
			stable = &data
		case types.ChannelBeta.String():
			beta = &data
		case types.ChannelAlpha.String():
			alpha = &data
		}
	}

	switch channel {
	case types.ChannelStable.String():
		if stable != nil {
			return stable, nil
		}
	case types.ChannelBeta.String():
		v, err := l.doCompare(stable, beta)
		if err != nil {
			return nil, err
		}
		if v != nil {
			return v, nil
		}
	case types.ChannelAlpha.String():
		v, err := l.doCompare(stable, beta, alpha)
		if err != nil {
			return nil, err
		}
		if v != nil {
			return v, nil
		}
	}

	return nil, misc.ResourceNotFound
}

func (l *VersionLogic) doCompare(args ...*LatestVersionInfo) (*LatestVersionInfo, error) {
	var r *LatestVersionInfo
	for i := range args {
		info := args[i]
		if info == nil {
			continue
		}
		if r == nil {
			r = info
		} else {
			result := l.comparator.Compare(r.VersionName, info.VersionName)
			if !result.Comparable {
				err := errors.New("failed to compare versions")
				l.logger.Error("Failed to compare versions",
					zap.Any("previous version", r),
					zap.Any("current version", info),
					zap.Error(err),
				)
				return nil, err
			}
			if result.Result == vercomp.Less {
				r = info
			}
		}
	}
	return r, nil
}

func (l *VersionLogic) cleanRootStoragePath(p string) string {
	rel := strings.TrimPrefix(p, l.storageLogic.RootDir)
	rel = strings.TrimPrefix(rel, string(os.PathSeparator))
	return strings.ReplaceAll(rel, string(os.PathSeparator), "/")
}

func (l *VersionLogic) GetDistributeURL(info *DistributeInfo) (string, error) {
	// 可以改成无状态的
	var (
		ctx    = context.Background()
		prefix = GConfig.Extra.DownloadRedirectPrefix
		rk     = ksuid.New().String()
	)

	val, err := sonic.MarshalString(info)
	if err != nil {
		l.logger.Error("Failed to marshal string",
			zap.Error(err),
		)
		return "", err
	}

	key := strings.Join([]string{misc.DispensePrefix, rk}, ":")

	_, err = l.rdb.Set(ctx, key, val, time.Minute*5).Result()
	if err != nil {
		l.logger.Error("failed to set distribute info",
			zap.Error(err),
		)
		return "", err
	}

	url := strings.Join([]string{prefix, rk}, "/")
	return url, nil
}

func (l *VersionLogic) GetDistributeLocation(ctx context.Context, rk string) (string, error) {
	key := strings.Join([]string{misc.DispensePrefix, rk}, ":")
	val, err := l.rdb.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	info := &DistributeInfo{}
	err = sonic.UnmarshalString(val, info)
	if err != nil {
		return "", err
	}

	url, err := l.distributeLogic.Distribute(info)
	if err != nil {
		return "", err
	}

	return url, nil
}

func (l *VersionLogic) fetchStorageInfoTuple(ctx context.Context, target, current int, system string, arch string) (*ent.Storage, *ent.Storage, error) {

	targetStorage, err := l.getFullUpdateStorageByCache(ctx, target, system, arch)
	if err != nil {
		return nil, nil, err
	}

	currentStorage, err := l.getFullUpdateStorageByCache(ctx, current, system, arch)
	if err != nil {
		return nil, nil, err
	}

	return targetStorage, currentStorage, nil
}

func (l *VersionLogic) getFullUpdateStorageByCache(ctx context.Context, versionId int, os, arch string) (*ent.Storage, error) {
	cg := l.cacheGroup
	cacheKey := cg.GetCacheKey(
		strconv.Itoa(versionId),
		os,
		arch,
	)
	val, err := cg.FullUpdateStorageCache.ComputeIfAbsent(cacheKey, func() (*ent.Storage, error) {
		return l.storageLogic.GetFullUpdateStorage(ctx, versionId, os, arch)
	})
	if err != nil {
		return nil, err
	}
	return *val, err
}

func (l *VersionLogic) getIncrementalInfoOrEmpty(ctx context.Context, target, current int, os, arch string) (*IncrementalUpdateInfo, error) {
	cacheKey := l.cacheGroup.GetCacheKey(
		strconv.Itoa(target),
		strconv.Itoa(current),
		os,
		arch,
	)
	val, err := l.cacheGroup.IncrementalUpdateInfoCache.ComputeIfAbsent(cacheKey, func() (*IncrementalUpdateInfo, error) {
		s, err := l.storageLogic.GetIncrementalUpdateStorage(ctx, target, current, os, arch)
		switch {
		case err != nil && ent.IsNotFound(err):
			return &IncrementalUpdateInfo{}, nil
		case err == nil:
			return &IncrementalUpdateInfo{Storage: s}, nil
		}
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	return *val, err
}

func renewMutex(ctx context.Context, mutex *redsync.Mutex) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ok, err := mutex.Extend()
			if !ok || err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}

}

func (l *VersionLogic) UpdateReleaseNote(ctx context.Context, param UpdateReleaseNoteDetailParam) error {
	return l.versionRepo.UpdateVersionReleaseNote(ctx, param.VersionID, param.ReleaseNoteDetail)
}

func (l *VersionLogic) UpdateCustomData(ctx context.Context, param UpdateReleaseNoteSummaryParam) error {
	return l.versionRepo.UpdateVersionCustomData(ctx, param.VersionID, param.ReleaseNoteSummary)
}
