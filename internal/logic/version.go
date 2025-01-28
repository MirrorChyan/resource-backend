package logic

import (
	"context"
	"errors"
	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/go-redsync/redsync/v4"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type VersionLogic struct {
	logger       *zap.Logger
	repo         *repo.Repo
	versionRepo  *repo.Version
	storageRepo  *repo.Storage
	storageLogic *StorageLogic
	rdb          *redis.Client
	sync         *redsync.Redsync
	cacheGroup   *cache.VersionCacheGroup
}

func NewVersionLogic(
	logger *zap.Logger,
	repo *repo.Repo,
	versionRepo *repo.Version,
	storageRepo *repo.Storage,
	storageLogic *StorageLogic,
	rdb *redis.Client,
	sync *redsync.Redsync,
	cacheGroup *cache.VersionCacheGroup,
) *VersionLogic {
	return &VersionLogic{
		logger:       logger,
		repo:         repo,
		versionRepo:  versionRepo,
		storageRepo:  storageRepo,
		storageLogic: storageLogic,
		rdb:          rdb,
		sync:         sync,
		cacheGroup:   cacheGroup,
	}
}

const (
	actualResourcePath = "resource"
	archiveZip         = "resource.zip"

	resourcePrefix = "Res"

	zipSuffix         = ".zip"
	tarGzSuffix       = ".tar.gz"
	specificSeparator = "$#@"
)

func (l *VersionLogic) NameExists(ctx context.Context, param VersionNameExistsParam) (bool, error) {
	return l.versionRepo.CheckVersionExistsByName(ctx, param.ResourceID, param.Name)
}

func (l *VersionLogic) GetLatest(ctx context.Context, resourceID string) (*ent.Version, error) {
	val, err := l.cacheGroup.VersionLatestCache.ComputeIfAbsent(resourceID, func() (*ent.Version, error) {
		return l.versionRepo.GetLatestVersion(ctx, resourceID)
	})
	if err != nil {
		return nil, err
	}
	return *val, err
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, error) {
	var number uint64 = 1
	latest, err := l.versionRepo.GetLatestVersion(ctx, param.ResourceID)
	if err != nil && !ent.IsNotFound(err) {
		l.logger.Error("version create failed",
			zap.Error(err),
		)
		return nil, err
	}

	if latest != nil {
		number = latest.Number + 1
	}

	// external closure variable
	var version *ent.Version

	err = l.repo.WithTx(ctx, func(tx *ent.Tx) error {
		var (
			err         error
			saveDir     string
			archivePath string
		)
		// version and storage must be consistent
		version, err = l.versionRepo.CreateVersionTx(ctx, tx, param.ResourceID, param.Name, number)
		if err != nil {
			l.logger.Error("Failed to create version",
				zap.String("resource id", param.ResourceID),
				zap.String("version name", param.Name),
				zap.Error(err),
			)
			return err
		}

		saveDir = l.storageLogic.BuildVersionResourceStorageDirPath(param.ResourceID, version.ID, param.OS, param.Arch)
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

		archivePath = l.storageLogic.BuildVersionResourceStoragePath(param.ResourceID, version.ID, param.OS, param.Arch)

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

		hashes, err := filehash.GetAll(saveDir)
		if err != nil {
			l.logger.Error("Failed to get file hashes",
				zap.String("version name", param.Name),
				zap.Error(err),
			)
			return err
		}

		_, err = l.storageLogic.CreateFullUpdateStorage(ctx, tx, version.ID, param.OS, param.Arch, archivePath, saveDir, hashes)
		if err != nil {
			l.logger.Error("Failed to create storage",
				zap.Error(err),
			)
			return err
		}

		return nil
	})

	l.doPostCreateResources(param.ResourceID)

	return version, nil
}

func (l *VersionLogic) doPostCreateResources(rid string) {
	l.cacheGroup.VersionLatestCache.Delete(rid)
}

func (l *VersionLogic) doProcessPatchOrFullUpdate(ctx context.Context, param ProcessUpdateParam) (string, error) {
	// if current version is not provided, we will download the full version
	var (
		err            error
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
			return "", err
		default:
			isFull = true
		}

	}

	if isFull {
		cacheKey := cacheGroup.GetCacheKey(
			param.OS,
			param.Arch,
			strconv.Itoa(param.TargetVersion.ID),
		)
		val, err := cacheGroup.FullUpdatePathCache.ComputeIfAbsent(cacheKey, func() (string, error) {
			return l.GetFullUpdatePackagePath(ctx, GetFullUpdatePackagePathParam{
				ResourceID: resourceID,
				VersionID:  param.TargetVersion.ID,
				OS:         param.OS,
				Arch:       param.Arch,
			})
		})
		if err != nil {
			return "", err
		}
		return *val, nil

	}

	result, err := l.GetIncrementalUpdatePackagePath(ctx, UpdateProcessInfo{
		ResourceID:       resourceID,
		CurrentVersionID: currentVersion.ID,
		TargetVersionID:  targetVersion.ID,
		OS:               param.OS,
		Arch:             param.Arch,
	})
	if err != nil {
		l.logger.Error("failed to get incremental update package path",
			zap.Error(err),
		)
		return "", err
	}

	return result, nil
}

func (l *VersionLogic) GetDownloadUrl(ctx context.Context, param ProcessUpdateParam) (string, error) {
	var (
		cfg = config.CFG
	)
	p, err := l.doProcessPatchOrFullUpdate(ctx, param)
	if err != nil {
		return "", err
	}

	rel := l.cleanPath(p)

	key := ksuid.New().String()
	sk := strings.Join([]string{resourcePrefix, key}, ":")

	_, err = l.rdb.Set(ctx, sk, rel, cfg.Extra.DownloadEffectiveTime).Result()
	if err != nil {
		return "", err
	}

	return strings.Join([]string{cfg.Extra.DownloadPrefix, key}, "/"), nil
}

func (l *VersionLogic) cleanPath(p string) string {
	rel := strings.TrimPrefix(p, l.storageLogic.RootDir)
	rel = strings.TrimPrefix(rel, string(os.PathSeparator))
	rel = strings.ReplaceAll(rel, string(os.PathSeparator), "/")
	return rel
}

func (l *VersionLogic) isPatchLoaded(ctx context.Context, cacheKey string) (string, bool, error) {
	result, err := l.rdb.Get(ctx, cacheKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return "", false, nil
	}

	if result != "" {
		r := strings.Split(result, specificSeparator)
		if len(r) > 2 {
			return "", false, errors.New("patch cache error")
		}

		if len(r) == 1 || r[1] == "" {
			return r[0], true, nil
		}

		return r[0], true, errors.New(r[1])
	}
	return "", false, nil
}

func (l *VersionLogic) LoadPatchInfo(ctx context.Context, cacheKey, p, e string) error {
	strings.Join([]string{"Load", cacheKey, e}, specificSeparator)
	_, err := l.rdb.Set(ctx, cacheKey, p, 0).Result()
	if err != nil {
		return err
	}
	return nil
}

func (l *VersionLogic) GetCacheGroup() *cache.VersionCacheGroup {
	return l.cacheGroup
}

func (l *VersionLogic) fetchStorageInfoTuple(ctx context.Context, target, current int, resOS string, resArch string) (*ent.Storage, *ent.Storage, error) {

	var (
		targetStorage  *ent.Storage
		currentStorage *ent.Storage
		ch             = make(chan *ent.Storage, 1)
	)
	defer close(ch)
	wg := errgroup.Group{}
	wg.Go(func() error {
		s, err := l.storageLogic.GetFullUpdateStorage(ctx, target, resOS, resArch)
		if err != nil {
			return err
		}
		ch <- s
		return nil
	})
	currentStorage, err := l.storageLogic.GetFullUpdateStorage(ctx, current, resOS, resArch)

	wge := wg.Wait()

	if err != nil || wge != nil {
		return nil, nil, err
	}

	targetStorage = <-ch

	return targetStorage, currentStorage, nil
}

func (l *VersionLogic) GetIncrementalUpdatePackagePath(ctx context.Context, param UpdateProcessInfo) (string, error) {
	return l.doGetIncrementalUpdatePackagePath(ctx, param)
}

func (l *VersionLogic) CreateIncrementalUpdatePackage(ctx context.Context, param UpdateProcessInfo) (string, error) {
	var (
		targetVersion  = strconv.Itoa(param.TargetVersionID)
		currentVersion = strconv.Itoa(param.CurrentVersionID)
		resourceID     = param.ResourceID

		mutexKey = strings.Join([]string{"Patch", resourceID, targetVersion, currentVersion}, ":")
		cacheKey = strings.Join([]string{"Load", resourceID, targetVersion, currentVersion}, ":")
	)

	val, done, err := l.isPatchLoaded(ctx, cacheKey)
	switch {
	case err != nil:
		return "", err
	case done:
		return val, nil
	}

	mutex := l.sync.NewMutex(mutexKey)

	if err := mutex.Lock(); err != nil {
		return "", err
	}
	defer func() {
		if ok, err := mutex.Unlock(); !ok || err != nil {
			l.logger.Error("Failed to unlock patch mutex")
		}
	}()

	val, done, err = l.isPatchLoaded(ctx, cacheKey)
	switch {
	case err != nil:
		return "", err
	case done:
		return val, nil
	}

	p, err := l.doCreateIncrementalUpdatePackage(ctx, param)

	var e string
	if err != nil {
		e = err.Error()
	}

	if err := l.LoadPatchInfo(ctx, cacheKey, p, e); err != nil {
		return "", err
	}

	return p, nil
}

func (l *VersionLogic) doGetIncrementalUpdatePackagePath(ctx context.Context, param UpdateProcessInfo) (string, error) {

	// find existing incremental update
	cacheKey := strings.Join([]string{
		param.OS,
		param.Arch,
		strconv.Itoa(param.CurrentVersionID),
		strconv.Itoa(param.TargetVersionID),
	}, ":")
	p, err := l.cacheGroup.IncrementalUpdatePathCache.ComputeIfAbsent(cacheKey, func() (string, error) {
		return l.storageLogic.GetIncrementalUpdatePath(ctx, param)
	})

	switch {
	case err != nil && !ent.IsNotFound(err):
		l.logger.Error("Failed to get incremental update package path",
			zap.Error(err),
		)
		return "", err
	case err == nil:
		return *p, nil
	default:
		// create not existed incremental update
	}

	packagePath, err := l.CreateIncrementalUpdatePackage(ctx, param)
	if err != nil {
		l.logger.Error("Failed to generate incremental update package",
			zap.Error(err),
		)
		return "", err
	}

	return packagePath, nil
}

func (l *VersionLogic) doCreateIncrementalUpdatePackage(ctx context.Context, param UpdateProcessInfo) (string, error) {

	var (
		resourceID  = param.ResourceID
		target      = param.TargetVersionID
		current     = param.CurrentVersionID
		resOS       = param.OS
		resArch     = param.Arch
		packagePath string
	)

	targetStorage, currentStorage, err := l.fetchStorageInfoTuple(ctx, target, current, resOS, resArch)
	if err != nil {
		l.logger.Error("Failed to get storage info",
			zap.Error(err),
		)
		return "", err
	}

	changes, err := patcher.CalculateDiff(targetStorage.FileHashes, currentStorage.FileHashes)
	if err != nil {
		l.logger.Error("Failed to calculate diff",
			zap.Error(err),
		)
		return "", err
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
		return "", err
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
		_, err = l.storageLogic.CreateIncrementalUpdateStorage(ctx, tx, target, current, resOS, resArch, packagePath)
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
		return "", err
	}

	return packagePath, nil
}

func (l *VersionLogic) GetFullUpdatePackagePath(ctx context.Context, param GetFullUpdatePackagePathParam) (string, error) {
	return l.storageLogic.GetFullUpdatePath(ctx, param.VersionID, param.OS, param.Arch)
}
