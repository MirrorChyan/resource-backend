package logic

import (
	"context"
	"errors"

	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/latestversion"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/lb"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/bytedance/sonic"
	"github.com/go-redsync/redsync/v4"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
)

type VersionLogic struct {
	logger             *zap.Logger
	repo               *repo.Repo
	versionRepo        *repo.Version
	storageRepo        *repo.Storage
	latestVersionLogic *LatestVersionLogic
	storageLogic       *StorageLogic
	rdb                *redis.Client
	sync               *redsync.Redsync
	cacheGroup         *cache.VersionCacheGroup
}

func NewVersionLogic(
	logger *zap.Logger,
	repo *repo.Repo,
	versionRepo *repo.Version,
	storageRepo *repo.Storage,
	latestVersionLogic *LatestVersionLogic,
	storageLogic *StorageLogic,
	rdb *redis.Client,
	sync *redsync.Redsync,
	cacheGroup *cache.VersionCacheGroup,
) *VersionLogic {
	return &VersionLogic{
		logger:             logger,
		repo:               repo,
		versionRepo:        versionRepo,
		storageRepo:        storageRepo,
		latestVersionLogic: latestVersionLogic,
		storageLogic:       storageLogic,
		rdb:                rdb,
		sync:               sync,
		cacheGroup:         cacheGroup,
	}
}

const (
	actualResourcePath = "resource"
	archiveZip         = "resource.zip"

	FullUpdateType        = "full"
	IncrementalUpdateType = "incremental"

	resourcePrefix = "Res"

	zipSuffix         = ".zip"
	tarGzSuffix       = ".tar.gz"
	specificSeparator = "$#@"
)

var (
	StorageInfoNotFound = errors.New("storage info not found")

	wrr = sync.OnceValue(func() *lb.WeightedRoundRobin {
		var (
			prefix  = config.CFG.Extra.DownloadPrefix
			servers []lb.Server
		)
		l := len(prefix)
		for i := 0; i < l; i += 2 {
			w, err := strconv.Atoi(prefix[i+1])
			if err != nil {
				continue
			}
			servers = append(servers, lb.Server{
				Url:    prefix[i],
				Weight: w,
			})
		}
		return lb.NewWeightedRoundRobin(servers)
	})
)

func (l *VersionLogic) GetRedisClient() *redis.Client {
	return l.rdb
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
	ver, err := l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.VersionName)
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

func (l *VersionLogic) GetLatestStableVersion(ctx context.Context, resID string) (*ent.Version, error) {
	cacheKey := l.cacheGroup.GetCacheKey(resID, version.ChannelStable.String())
	val, err := l.cacheGroup.VersionLatestCache.ComputeIfAbsent(cacheKey, func() (*ent.Version, error) {
		return l.latestVersionLogic.GetLatestStableVersion(ctx, resID)
	})
	if err != nil {
		return nil, err
	}

	return *val, err
}

func (l *VersionLogic) GetLatestBetaVersion(ctx context.Context, resID string) (*ent.Version, error) {
	cacheKey := l.cacheGroup.GetCacheKey(resID, version.ChannelBeta.String())
	val, err := l.cacheGroup.VersionLatestCache.ComputeIfAbsent(cacheKey, func() (*ent.Version, error) {
		return l.latestVersionLogic.GetLatestBetaVersion(ctx, resID)
	})
	if err != nil {
		return nil, err
	}

	return *val, err
}

func (l *VersionLogic) GetLatestAlphaVersion(ctx context.Context, resID string) (*ent.Version, error) {
	cacheKey := l.cacheGroup.GetCacheKey(resID, version.ChannelAlpha.String())
	val, err := l.cacheGroup.VersionLatestCache.ComputeIfAbsent(cacheKey, func() (*ent.Version, error) {
		return l.latestVersionLogic.GetLatestAlphaVersion(ctx, resID)
	})
	if err != nil {
		return nil, err
	}

	return *val, err
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

func (l *VersionLogic) CreateVersion(ctx context.Context, resID, channel, name string) (*ent.Version, error) {
	number, err := l.GetVersionNumber(ctx, resID)
	if err != nil {
		l.logger.Error("Failed to get version number",
			zap.String("resource id", resID),
			zap.Error(err),
		)
		return nil, err
	}

	verChannel := l.GetVersionChannel(channel)

	var ver *ent.Version
	err = l.repo.WithTx(ctx, func(tx *ent.Tx) error {
		ver, err = l.versionRepo.CreateVersion(ctx, tx, resID, verChannel, name, number)
		if err != nil {
			l.logger.Error("Failed to create new version",
				zap.String("resource id", resID),
				zap.String("channel", channel),
				zap.String("version name", name),
				zap.Error(err),
			)
			return err
		}

		err = l.latestVersionLogic.UpdateLatestVersion(ctx, tx, resID, latestversion.Channel(verChannel), ver)
		if err != nil {
			l.logger.Error("Failed to update latest version",
				zap.String("resource id", resID),
				zap.String("channel", channel),
				zap.String("version name", name),
				zap.Error(err),
			)
			return err
		}

		return nil
	})

	if err != nil {
		l.logger.Error("Failed to create new version",
			zap.Error(err),
		)
		return nil, err
	}

	// clear old version resources
	go l.clearOldStorages(resID, verChannel, ver.ID, ver.Name)

	return ver, nil
}

func (l *VersionLogic) clearOldStorages(resID string, channel version.Channel, verID int, verName string) {
	ctx := context.Background()
	maxRetries := 3
	var err error

	for i := 0; i < maxRetries; i++ {
		err = l.storageLogic.ClearOldStorages(ctx, resID, channel, verID)
		if err == nil {
			break
		}
		l.logger.Warn("Failed to clear old storages, retrying...",
			zap.Int("retry count", i+1),
			zap.String("resource id", resID),
			zap.String("channel", channel.String()),
			zap.String("latest version name", verName),
			zap.Error(err),
		)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		l.logger.Error("Failed to clear old storages after multiple retries",
			zap.String("resource id", resID),
			zap.String("channel", channel.String()),
			zap.String("latest version name", verName),
			zap.Error(err),
		)
	}
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, error) {
	ver, err := l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.Name)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	} else if ent.IsNotFound(err) {
		ver, err = l.CreateVersion(ctx, param.ResourceID, param.Channel, param.Name)
		if err != nil {
			l.logger.Error("Failed to create new version",
				zap.String("resource id", param.ResourceID),
				zap.String("channel", param.Channel),
				zap.String("version name", param.Name),
				zap.Error(err),
			)
			return nil, err
		}

		l.doPostCreateResources(param.ResourceID, ver.Channel.String())
	}

	err = l.repo.WithTx(ctx, func(tx *ent.Tx) error {
		var (
			err         error
			saveDir     string
			archivePath string
		)

		saveDir = l.storageLogic.BuildVersionResourceStorageDirPath(param.ResourceID, ver.ID, param.OS, param.Arch)
		if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
			l.logger.Error("Failed to create storage directory",
				zap.String("directory", saveDir),
				zap.Error(err),
			)
			return err
		}

		switch {
		case strings.HasSuffix(param.UploadArchivePath, zipSuffix):
			err = archive.UnpackZip(param.UploadArchivePath, saveDir)
		case strings.HasSuffix(param.UploadArchivePath, tarGzSuffix):
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
				zap.String("version name", param.Name),
				zap.Error(err),
			)
			return err
		}

		archivePath = l.storageLogic.BuildVersionResourceStoragePath(param.ResourceID, ver.ID, param.OS, param.Arch)

		if strings.HasSuffix(param.UploadArchivePath, zipSuffix) {
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

		packageSHA256, err := filehash.Calculate(archivePath)
		if err != nil {
			l.logger.Error("Failed to calculate full update package hash",
				zap.String("resource id", param.ResourceID),
				zap.String("version name", param.Name),
				zap.String("os", param.OS),
				zap.String("arch", param.Arch),
				zap.Error(err),
			)
			return err
		}

		hashes, err := filehash.GetAll(saveDir)
		if err != nil {
			l.logger.Error("Failed to get file hashes",
				zap.String("version name", param.Name),
				zap.Error(err),
			)
			return err
		}

		_, err = l.storageLogic.CreateFullUpdateStorage(ctx, tx, ver.ID, param.OS, param.Arch, archivePath, packageSHA256, saveDir, hashes)
		if err != nil {
			l.logger.Error("Failed to create storage",
				zap.Error(err),
			)
			return err
		}

		return nil
	})

	if err != nil {
		l.logger.Error("Failed to create version",
			zap.Error(err),
		)
		return nil, err
	}

	return ver, nil
}

func (l *VersionLogic) doPostCreateResources(resID, channel string) {
	cacheKey := l.cacheGroup.GetCacheKey(resID, channel)
	l.cacheGroup.VersionLatestCache.Delete(cacheKey)
}

func (l *VersionLogic) doProcessPatchOrFullUpdate(ctx context.Context, param ProcessUpdateParam) (packagePath, packageSHA256, updateType string, err error) {
	// if current version is not provided, we will download the full version
	var (
		cacheGroup     = l.cacheGroup
		isFull         = param.CurrentVersionName == ""
		resourceID     = param.ResourceID
		targetVersion  = param.TargetVersion
		currentVersion *ent.Version
	)

	// full update

	if !isFull {
		var cacheKey = cacheGroup.GetCacheKey(param.ResourceID, param.CurrentVersionName)
		val, err := cacheGroup.VersionNameCache.ComputeIfAbsent(cacheKey, func() (*ent.Version, error) {
			return l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.CurrentVersionName)
		})
		switch {
		case err == nil:
			currentVersion = *val
		case !ent.IsNotFound(err):
			return "", "", "", err
		default:
			isFull = true
		}

	}

	if isFull {
		fullUpdateStorage, err := l.getFullUpdateStorageByCache(ctx, param.TargetVersion.ID, param.OS, param.Arch)
		if err != nil {
			if ent.IsNotFound(err) {
				return "", "", "", StorageInfoNotFound
			}
			l.logger.Error("failed to get full storage info",
				zap.Error(err),
			)
			return "", "", "", err
		}

		// TODO: this part is going to be removed when the data of the old version has a hash value in the future
		if fullUpdateStorage.PackageHashSha256 == "" {
			fullUpdateStorage.PackageHashSha256, err = filehash.Calculate(fullUpdateStorage.PackagePath)
			if err != nil {
				l.logger.Error("failed to calculate full update package hash",
					zap.String("resource id", resourceID),
					zap.String("version name", param.TargetVersion.Name),
					zap.String("os", param.OS),
					zap.String("arch", param.Arch),
					zap.Error(err),
				)
			}

			err = l.storageLogic.SetPackageSHA256(ctx, fullUpdateStorage.ID, fullUpdateStorage.PackageHashSha256)
			if err != nil {
				l.logger.Error("failed to set full udpate package hash",
					zap.Int("storage id", fullUpdateStorage.ID),
					zap.Error(err),
				)
				return "", "", "", err
			}
		}

		packagePath = fullUpdateStorage.PackagePath
		packageSHA256 = fullUpdateStorage.PackageHashSha256
		updateType = FullUpdateType

		return packagePath, packageSHA256, updateType, nil
	}

	info := ActualUpdateProcessInfo{
		Info: UpdateProcessInfo{
			ResourceID:       resourceID,
			CurrentVersionID: currentVersion.ID,
			TargetVersionID:  targetVersion.ID,
			OS:               param.OS,
			Arch:             param.Arch,
		},
		Target:  nil,
		Current: nil,
	}

	info.Target, info.Current, err = l.fetchStorageInfoTuple(ctx, targetVersion.ID, currentVersion.ID, param.OS, param.Arch)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", "", "", StorageInfoNotFound
		}
		l.logger.Error("failed to get storage info",
			zap.Error(err),
		)
		return "", "", "", err
	}

	incrementalUpdatePackage, err := l.GetIncrementalUpdatePackage(ctx, info)
	if err != nil {
		l.logger.Error("failed to get incremental update package path",
			zap.Error(err),
		)
		return "", "", "", err
	}

	packagePath = incrementalUpdatePackage.Path
	packageSHA256 = incrementalUpdatePackage.SHA256
	updateType = IncrementalUpdateType

	return packagePath, packageSHA256, updateType, nil
}

func (l *VersionLogic) GetUpdateInfo(ctx context.Context, oriented bool, cdk string, param ProcessUpdateParam) (url, packageSHA256, updateType string, err error) {
	var (
		cfg = config.CFG
	)
	// path is the download path, type is the update type
	packagePath, packageSHA256, updateType, err := l.doProcessPatchOrFullUpdate(ctx, param)
	if err != nil {
		return "", "", "", err
	}

	rel := l.cleanPath(packagePath)

	key := ksuid.New().String()
	sk := strings.Join([]string{resourcePrefix, key}, ":")

	value, err := sonic.Marshal(map[string]string{
		"cdk":  cdk,
		"path": rel,
	})
	if err != nil {
		return "", "", "", err
	}

	_, err = l.rdb.Set(ctx, sk, value, cfg.Extra.DownloadEffectiveTime).Result()
	if err != nil {
		return "", "", "", err
	}

	var prefix string

	// FIXME temporary use
	if oriented && len(cfg.Extra.DownloadPrefix) > 2 {
		prefix = cfg.Extra.DownloadPrefix[2]
	} else {
		prefix = wrr().Next()
	}

	url = strings.Join([]string{prefix, key}, "/")

	return url, packageSHA256, updateType, nil
}

func (l *VersionLogic) cleanPath(p string) string {
	rel := strings.TrimPrefix(p, l.storageLogic.RootDir)
	rel = strings.TrimPrefix(rel, string(os.PathSeparator))
	rel = strings.ReplaceAll(rel, string(os.PathSeparator), "/")
	return rel
}

func (l *VersionLogic) isPatchLoaded(ctx context.Context, cacheKey string) (UpdatePackage, bool, error) {
	result, err := l.rdb.Get(ctx, cacheKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return UpdatePackage{}, false, nil
	}

	if result != "" {
		r := strings.Split(result, specificSeparator)
		if len(r) > 2 {
			return UpdatePackage{}, false, errors.New("patch cache error")
		}

		if len(r) == 1 || r[1] == "" {
			var p UpdatePackage
			err = sonic.Unmarshal([]byte(r[0]), &p)
			if err != nil {
				return UpdatePackage{}, false, err
			}

			return p, true, nil
		}

		var p UpdatePackage
		err = sonic.Unmarshal([]byte(r[0]), &p)
		if err != nil {
			return UpdatePackage{}, false, err
		}

		return p, true, errors.New(r[1])
	}
	return UpdatePackage{}, false, nil
}

func (l *VersionLogic) StorePatchInfo(ctx context.Context, cacheKey string, p UpdatePackage, e string) error {
	pData, err := sonic.Marshal(p)
	if err != nil {
		return err
	}

	val := strings.Join([]string{string(pData), e}, specificSeparator)
	_, err = l.rdb.Set(ctx, cacheKey, val, time.Minute*5).Result()
	if err != nil {
		return err
	}
	return nil
}

func (l *VersionLogic) GetCacheGroup() *cache.VersionCacheGroup {
	return l.cacheGroup
}

func (l *VersionLogic) fetchStorageInfoTuple(ctx context.Context, target, current int, resOS string, resArch string) (*ent.Storage, *ent.Storage, error) {

	targetStorage, err := l.getFullUpdateStorageByCache(ctx, target, resOS, resArch)
	if err != nil {
		return nil, nil, err
	}

	currentStorage, err := l.getFullUpdateStorageByCache(ctx, current, resOS, resArch)
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

func (l *VersionLogic) getIncrementalUpdateStorageByCache(ctx context.Context, targetVerID, currentVerID int, os, arch string) (*ent.Storage, error) {
	cacheKey := l.cacheGroup.GetCacheKey(
		strconv.Itoa(targetVerID),
		strconv.Itoa(currentVerID),
		os,
		arch,
	)
	val, err := l.cacheGroup.IncrementalUpdateStorageCache.ComputeIfAbsent(cacheKey, func() (*ent.Storage, error) {
		return l.storageLogic.GetIncrementalUpdateStorage(ctx, targetVerID, currentVerID, os, arch)
	})
	if err != nil {
		return nil, err
	}
	return *val, err
}

func (l *VersionLogic) CreateIncrementalUpdatePackage(ctx context.Context, info ActualUpdateProcessInfo) (UpdatePackage, error) {
	var (
		param          = info.Info
		targetVersion  = strconv.Itoa(param.TargetVersionID)
		currentVersion = strconv.Itoa(param.CurrentVersionID)
		resourceID     = param.ResourceID

		mutexKey = strings.Join([]string{"Patch", resourceID, targetVersion, currentVersion}, ":")
		cacheKey = strings.Join([]string{"Load", resourceID, targetVersion, currentVersion}, ":")
	)

	// fast return avoid flooding the entire service
	val, done, err := l.isPatchLoaded(ctx, cacheKey)
	switch {
	case err != nil:
		return UpdatePackage{}, err
	case done:
		return UpdatePackage{
			Path:   val.Path,
			SHA256: val.SHA256,
		}, nil
	}

	mutex := l.sync.NewMutex(mutexKey, redsync.WithExpiry(10*time.Second))

	if err := mutex.Lock(); err != nil {
		return UpdatePackage{}, err
	}

	c, cancel := context.WithCancel(ctx)
	defer cancel()

	go renewMutex(c, mutex)

	defer func() {
		if ok, err := mutex.Unlock(); !ok || err != nil {
			l.logger.Error("Failed to unlock patch mutex")
		}
	}()

	val, done, err = l.isPatchLoaded(ctx, cacheKey)
	switch {
	case err != nil:
		return UpdatePackage{}, err
	case done:
		return UpdatePackage{
			Path:   val.Path,
			SHA256: val.SHA256,
		}, nil
	}

	packagePath, packageSHA256, err := l.doCreateIncrementalUpdatePackage(ctx, info)

	var e string
	if err != nil {
		e = err.Error()
	}

	if err := l.StorePatchInfo(ctx, cacheKey, UpdatePackage{
		Path:   packagePath,
		SHA256: packageSHA256,
	}, e); err != nil {
		return UpdatePackage{}, err
	}

	return UpdatePackage{
		Path:   packagePath,
		SHA256: packageSHA256,
	}, nil
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

func (l *VersionLogic) GetIncrementalUpdatePackage(ctx context.Context, info ActualUpdateProcessInfo) (UpdatePackage, error) {

	var (
		param = info.Info
	)

	// find existing incremental update
	incrementalUpdateStorage, err := l.getIncrementalUpdateStorageByCache(ctx, param.TargetVersionID, param.CurrentVersionID, param.OS, param.Arch)
	switch {
	case err != nil && !ent.IsNotFound(err):
		l.logger.Error("Failed to get incremental update package path",
			zap.Error(err),
		)
		return UpdatePackage{}, err
	case err == nil:
		// TODO: this part is going to be removed when the data of the old version has a hash value in the future
		if incrementalUpdateStorage.PackageHashSha256 == "" {
			incrementalUpdateStorage.PackageHashSha256, err = filehash.Calculate(incrementalUpdateStorage.PackagePath)
			if err != nil {
				l.logger.Error("Failed to calculate incremental update package hash",
					zap.Int("storage id", incrementalUpdateStorage.ID),
					zap.Error(err),
				)
			}

			err = l.storageLogic.SetPackageSHA256(ctx, incrementalUpdateStorage.ID, incrementalUpdateStorage.PackageHashSha256)
			if err != nil {
				l.logger.Error("Failed to set incremental update package hash",
					zap.Int("storage id", incrementalUpdateStorage.ID),
					zap.Error(err),
				)
			}
		}

		return UpdatePackage{
			Path:   incrementalUpdateStorage.PackagePath,
			SHA256: incrementalUpdateStorage.PackageHashSha256,
		}, nil
	default:
		// create not existed incremental update
	}

	incrementalUpdatePackage, err := l.CreateIncrementalUpdatePackage(ctx, info)
	if err != nil {
		l.logger.Error("Failed to generate incremental update package",
			zap.Error(err),
		)
		return UpdatePackage{}, err
	}

	return incrementalUpdatePackage, nil
}

func (l *VersionLogic) doCreateIncrementalUpdatePackage(ctx context.Context, info ActualUpdateProcessInfo) (packagePath, packageSHA256 string, err error) {

	var (
		param      = info.Info
		resourceID = param.ResourceID
		target     = param.TargetVersionID
		current    = param.CurrentVersionID
		resOS      = param.OS
		resArch    = param.Arch

		targetStorage  = info.Target
		currentStorage = info.Current
	)

	changes, err := patcher.CalculateDiff(targetStorage.FileHashes, currentStorage.FileHashes)
	if err != nil {
		l.logger.Error("Failed to calculate diff",
			zap.Error(err),
		)
		return "", "", err
	}

	patchDir := l.storageLogic.BuildVersionPatchStorageDirPath(resourceID, target, resOS, resArch)

	var (
		resourceDir = targetStorage.ResourcePath
	)

	patchName, err := patcher.Generate(strconv.Itoa(current), resourceDir, patchDir, changes)

	if err != nil {
		l.logger.Error("Failed to generate patch package",
			zap.Error(err),
		)
		return "", "", err
	}

	err = l.repo.WithTx(ctx, func(tx *ent.Tx) error {

		tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
			return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
				// Code before the actual rollback.

				if err := os.RemoveAll(packagePath); err != nil {
					l.logger.Error("Failed to remove patch package",
						zap.Error(err),
					)
				}

				err := next.Rollback(ctx, tx)
				// Code after the transaction was rolled back.

				return err
			})
		})

		packagePath = filepath.Join(patchDir, patchName)
		packageSHA256, err := filehash.Calculate(packagePath)
		if err != nil {
			l.logger.Error("Failed to calculate incremental update package hash",
				zap.String("resource id", info.Info.ResourceID),
				zap.Int("target version id", info.Info.TargetVersionID),
				zap.Int("current version id", info.Info.CurrentVersionID),
				zap.String("os", info.Info.OS),
				zap.String("arch", info.Info.Arch),
				zap.Error(err),
			)
			return err
		}
		_, err = l.storageLogic.CreateIncrementalUpdateStorage(ctx, tx, target, current, resOS, resArch, packagePath, packageSHA256)
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
		return "", "", err
	}

	return packagePath, packageSHA256, nil
}

func (l *VersionLogic) UpdateReleaseNote(ctx context.Context, param UpdateReleaseNoteDetailParam) error {
	return l.versionRepo.UpdateVersionReleaseNote(ctx, param.VersionID, param.ReleaseNoteDetail)
}

func (l *VersionLogic) UpdateCustomData(ctx context.Context, param UpdateReleaseNoteSummaryParam) error {
	return l.versionRepo.UpdateVersionCustomData(ctx, param.VersionID, param.ReleaseNoteSummary)
}
