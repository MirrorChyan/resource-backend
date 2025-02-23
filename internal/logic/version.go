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
	comparator      *vercomp.VersionComparator
	taskQueue       *tasks.TaskQueue
	rdb             *redis.Client
	sync            *redsync.Redsync
	cacheGroup      *cache.VersionCacheGroup
}

func NewVersionLogic(
	logger *zap.Logger,
	repo *repo.Repo,
	versionRepo *repo.Version,
	rawQuery *repo.RawQuery,
	verComparator *vercomp.VersionComparator,
	storageLogic *StorageLogic,
	rdb *redis.Client,
	sync *redsync.Redsync,
	taskQueue *tasks.TaskQueue,
	cacheGroup *cache.VersionCacheGroup,
	distributeLogic *dispense.DistributeLogic,
) *VersionLogic {
	l := &VersionLogic{
		logger:          logger,
		repo:            repo,
		versionRepo:     versionRepo,
		storageLogic:    storageLogic,
		comparator:      verComparator,
		rawQuery:        rawQuery,
		taskQueue:       taskQueue,
		distributeLogic: distributeLogic,
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

func (l *VersionLogic) GetCacheGroup() *cache.VersionCacheGroup {
	return l.cacheGroup
}

func (l *VersionLogic) GetVersionChannel(channel string) version.Channel {
	switch channel {
	case version.ChannelStable.String():
		return version.ChannelStable
	case version.ChannelBeta.String():
		return version.ChannelBeta
	case version.ChannelAlpha.String():
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

	ver, err := l.versionRepo.CreateVersion(ctx, resourceId, verChannel, name, number)

	return ver, nil
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, error) {
	var (
		resourceId  = param.ResourceID
		versionName = param.Name
		system      = param.OS
		arch        = param.Arch
		channel     = param.Channel
	)

	aspect := func() (*ent.Version, error) {

		ver, err := l.LoadStoreNewVersionTx(ctx, resourceId, versionName, channel)
		if err != nil {
			return nil, err
		}

		return ver, l.repo.WithTx(ctx, func(tx *ent.Tx) error {
			var (
				err         error
				saveDir     string
				archivePath string
			)

			saveDir = l.storageLogic.BuildVersionResourceStorageDirPath(resourceId, ver.ID, system, arch)
			if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
				l.logger.Error("Failed to create storage directory",
					zap.String("directory", saveDir),
					zap.Error(err),
				)
				return err
			}

			switch {
			case strings.HasSuffix(param.UploadArchivePath, misc.ZipSuffix):
				err = archive.UnpackZip(param.UploadArchivePath, saveDir)
			case strings.HasSuffix(param.UploadArchivePath, misc.TarGzSuffix):
				err = archive.UnpackTarGz(param.UploadArchivePath, saveDir)
			default:
				l.logger.Error("Unknown archive extension",
					zap.String("archive path", param.UploadArchivePath),
				)
				return errors.New("unknown archive extension")
			}

			tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
				return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
					// Code before the actual rollback.

					if e := os.RemoveAll(saveDir); e != nil {
						l.logger.Error("Failed to remove storage directory",
							zap.Error(e),
						)
					}

					err := next.Rollback(ctx, tx)
					// Code after the transaction was rolled back.

					return err
				})
			})

			if err != nil {
				l.logger.Error("Failed to unpack file",
					zap.String("version name", versionName),
					zap.Error(err),
				)
				return err
			}

			archivePath = l.storageLogic.BuildVersionResourceStoragePath(resourceId, ver.ID, system, arch)

			if strings.HasSuffix(param.UploadArchivePath, misc.ZipSuffix) {
				err = fileops.MoveFile(param.UploadArchivePath, archivePath)
				if err != nil {
					l.logger.Error("Failed to move archive file",
						zap.String("origin path", param.UploadArchivePath),
						zap.String("destination path", archivePath),
						zap.Error(err),
					)
					return err
				}
			} else {
				if err = archive.CompressToZip(saveDir, archivePath); err != nil {
					l.logger.Error("Failed to compress to zip",
						zap.String("src dir", saveDir),
						zap.String("dst file", archivePath),
						zap.Error(err),
					)
					return err
				}

			}

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

			hashes, err := filehash.GetAll(saveDir)
			if err != nil {
				l.logger.Error("Failed to get file hashes",
					zap.String("version name", versionName),
					zap.Error(err),
				)
				return err
			}

			_, err = l.storageLogic.CreateFullUpdateStorage(ctx, tx, ver.ID, system, arch, archivePath, packageHash, saveDir, hashes)
			if err != nil {
				l.logger.Error("Failed to create storage",
					zap.Error(err),
				)
				return err
			}
			return nil
		})
	}

	v, err := aspect()

	if err != nil {
		// do error callback
		go l.doWebhookNotify(resourceId, versionName, param.Channel, system, arch, false)
		l.logger.Error("Failed to create version",
			zap.Error(err),
		)
		return nil, err
	}

	l.doPostCreateResources(resourceId)

	go l.doWebhookNotify(resourceId, versionName, v.Channel.String(), system, arch, true)

	return v, nil
}

func (l *VersionLogic) LoadStoreNewVersionTx(ctx context.Context, resourceId, versionName, channel string) (*ent.Version, error) {
	var (
		ver      *ent.Version
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

	dest := filepath.Join(patchDir, patchName)

	err = l.repo.WithTx(ctx, func(tx *ent.Tx) (err error) {

		tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
			return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
				// Code before the actual rollback.
				if err := os.RemoveAll(dest); err != nil {
					l.logger.Error("Failed to remove patch package",
						zap.Error(err),
					)
				}
				err := next.Rollback(ctx, tx)
				// Code after the transaction was rolled back.
				return err
			})
		})

		hashes, err := filehash.Calculate(dest)
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
		_, err = l.storageLogic.CreateIncrementalUpdateStorage(ctx, tx, target, current, system, resArch, dest, hashes)
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
	info := (*val).LatestVersionInfo
	if info != nil {
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
		case misc.TypeStable:
			stable = &data
		case misc.TypeBeta:
			beta = &data
		case misc.TypeAlpha:
			alpha = &data
		}
	}

	switch channel {
	case misc.TypeStable:
		if stable != nil {
			return stable, nil
		}
	case misc.TypeBeta:
		v, err := l.doCompare(stable, beta)
		if err != nil {
			return nil, err
		}
		if v != nil {
			return v, nil
		}
	case misc.TypeAlpha:
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
				r1, _ := sonic.MarshalString(r)
				r2, _ := sonic.MarshalString(info)
				l.logger.Error("Failed to compare versions",
					zap.String("previous version", r1),
					zap.String("current version", r2),
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

func (l *VersionLogic) cleanStoragePath(p string) string {
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
