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
	"github.com/MirrorChyan/resource-backend/internal/pkg/stg"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/go-redsync/redsync/v4"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type VersionLogic struct {
	logger      *zap.Logger
	versionRepo *repo.Version
	storageRepo *repo.Storage
	storage     *stg.Storage
	rdb         *redis.Client
	sync        *redsync.Redsync
	cacheGroup  *cache.VersionCacheGroup
}

func NewVersionLogic(
	logger *zap.Logger,
	versionRepo *repo.Version,
	storageRepo *repo.Storage,
	storage *stg.Storage,
	redSync *redsync.Redsync,
	rdb *redis.Client,
	cacheGroup *cache.VersionCacheGroup,
) *VersionLogic {
	return &VersionLogic{
		logger:      logger,
		versionRepo: versionRepo,
		storageRepo: storageRepo,
		storage:     storage,
		sync:        redSync,
		rdb:         rdb,
		cacheGroup:  cacheGroup,
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
	return l.versionRepo.GetLatestVersion(ctx, resourceID)
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, error) {
	var number uint64 = 1
	latest, err := l.versionRepo.GetLatestVersion(ctx, param.ResourceID)
	if err != nil && ent.IsNotFound(err) {
		l.logger.Error("Failed to query latest version",
			zap.Error(err),
		)
		return nil, err
	}
	if latest != nil {
		number = latest.Number + 1
	}

	var ver *ent.Version

	err = l.versionRepo.WithTx(ctx, func(tx *ent.Tx) error {
		ver, err = l.versionRepo.CreateVersion(ctx, tx, param.ResourceID, param.Name, number)
		if err != nil {
			l.logger.Error("Failed to start transaction",
				zap.Error(err),
			)
			return err
		}

		versionDir := l.storage.VersionDir(param.ResourceID, ver.ID)
		saveDir := filepath.Join(versionDir, actualResourcePath)
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

				if err := os.RemoveAll(saveDir); err != nil {
					l.logger.Error("Failed to remove storage directory",
						zap.Error(err),
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

		archivePath := filepath.Join(versionDir, archiveZip)
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
			err = archive.CompressToZip(saveDir, archivePath)
			if err != nil {
				l.logger.Error("Failed to compress to zip",
					zap.String("src dir", saveDir),
					zap.String("dst file", archivePath),
					zap.Error(err),
				)
				return err
			}

		}

		fileHashes, err := filehash.GetAll(saveDir)
		if err != nil {
			l.logger.Error("Failed to get file hashes",
				zap.String("version name", param.Name),
				zap.Error(err),
			)
			return err
		}
		ver, err = l.versionRepo.SetVersionFileHashesByOne(ctx, tx, ver, fileHashes)
		if err != nil {
			l.logger.Error("Failed to add file hashes to version",
				zap.Error(err),
			)
			return err
		}

		s, err := l.storageRepo.CreateStorage(ctx, tx, saveDir)
		if err != nil {
			l.logger.Error("Failed to create storage",
				zap.Error(err),
			)
			return err
		}

		ver, err = l.versionRepo.SetVersionStorageByOne(ctx, tx, ver, s)
		if err != nil {
			l.logger.Error("Failed to add storage to version",
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
		return nil, err
	}

	l.cacheGroup.VersionLatestCache.Delete(param.ResourceID)

	return ver, nil
}

func (l *VersionLogic) GetByName(ctx context.Context, param GetVersionByNameParam) (*ent.Version, error) {
	return l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.Name)
}

func (l *VersionLogic) ProcessPatchOrFullUpdate(ctx context.Context, param ProcessUpdateParam) (string, error) {
	// if current version is not provided, we will download the full version
	var (
		current    *ent.Version
		err        error
		isFull     = param.CurrentVersionName == ""
		resourceID = param.ResourceID
	)

	// full update
	if isFull {
		return l.GetResourcePath(GetResourcePathParam{
			ResourceID: resourceID,
			VersionID:  param.LatestVersion.ID,
		}), nil
	}

	key := strings.Join([]string{resourceID, param.CurrentVersionName}, ":")

	val, err := l.cacheGroup.VersionNameCache.ComputeIfAbsent(key, func() (*ent.Version, error) {
		return l.GetByName(ctx, GetVersionByNameParam{
			ResourceID: resourceID,
			Name:       param.CurrentVersionName,
		})
	})
	if err != nil {
		if !ent.IsNotFound(err) {
			l.logger.Error("Failed to get current version",
				zap.Error(err),
			)
			return "", err
		}
		isFull = true
	}
	current = *val
	var (
		currentVersionID         = current.ID
		currentVersionFileHashes = current.FileHashes
	)

	// incremental update
	patchPath, err := l.GetPatchPath(ctx, GetVersionPatchParam{
		ResourceID:               resourceID,
		TargetVersionID:          param.LatestVersion.ID,
		TargetVersionFileHashes:  param.LatestVersion.FileHashes,
		CurrentVersionID:         currentVersionID,
		CurrentVersionFileHashes: currentVersionFileHashes,
	})

	if err != nil {
		l.logger.Error("Failed to get patch",
			zap.String("resource id", resourceID),
			zap.Int("target version id", param.LatestVersion.ID),
			zap.Int("current version id", currentVersionID),
			zap.Error(err),
		)
	}

	return patchPath, nil
}

func (l *VersionLogic) GetDownloadUrl(ctx context.Context, param ProcessUpdateParam) (string, error) {
	var (
		cfg = config.CFG
	)
	p, err := l.ProcessPatchOrFullUpdate(ctx, param)
	if err != nil {
		return "", err
	}

	rel := strings.TrimPrefix(p, l.storage.RootDir)
	rel = strings.TrimPrefix(rel, string(os.PathSeparator))
	rel = strings.ReplaceAll(rel, string(os.PathSeparator), "/")

	key := ksuid.New().String()
	sk := strings.Join([]string{resourcePrefix, key}, ":")
	_, err = l.rdb.Set(ctx, sk, rel, cfg.Extra.DownloadEffectiveTime).Result()
	if err != nil {
		return "", err
	}
	return strings.Join([]string{cfg.Extra.DownloadPrefix, key}, "/"), nil
}

func (l *VersionLogic) GetResourcePath(param GetResourcePathParam) string {
	return l.storage.ResourcePath(param.ResourceID, param.VersionID)
}

func (l *VersionLogic) GetPatchPath(ctx context.Context, param GetVersionPatchParam) (string, error) {
	exists, err := l.storage.PatchExists(param.ResourceID, param.TargetVersionID, param.CurrentVersionID)
	if err != nil {
		l.logger.Error("Failed to check patch file exists",
			zap.String("resource ID", param.ResourceID),
			zap.Int("target version ID", param.TargetVersionID),
			zap.Int("current version ID", param.CurrentVersionID),
			zap.Error(err),
		)
		return "", err
	}

	if exists {
		patchPath := l.storage.PatchPath(param.ResourceID, param.TargetVersionID, param.CurrentVersionID)
		return patchPath, nil
	}

	var (
		target   = strconv.Itoa(param.TargetVersionID)
		origin   = strconv.Itoa(param.CurrentVersionID)
		mutexKey = strings.Join([]string{"Patch", param.ResourceID, origin, target}, ":")

		cacheKey = strings.Join([]string{"Load", param.ResourceID, origin, target}, ":")
	)

	val, done, err := l.isPatchLoaded(ctx, cacheKey)
	l.logger.Info("val",
		zap.String("val", val),
		zap.Bool("done", done),
		zap.Error(err),
	)
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
			l.logger.Error("Failed to unlock patch mutex",
				zap.String("resource id", param.ResourceID),
				zap.Int("target version id", param.TargetVersionID),
				zap.Int("current version id", param.CurrentVersionID),
				zap.Error(err),
			)
		}
	}()

	val, done, err = l.isPatchLoaded(ctx, cacheKey)
	l.logger.Info("val",
		zap.String("val", val),
		zap.Bool("done", done),
		zap.Error(err),
	)
	switch {
	case err != nil:
		return "", err
	case done:
		return val, nil
	}

	p, err := l.doGetPatchPath(ctx, param, err)

	var e string
	if err != nil {
		e = err.Error()
	}

	if err := l.LoadPatchInfo(ctx, cacheKey, p, e); err != nil {
		return "", err
	}

	return p, nil
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

func (l *VersionLogic) doGetPatchPath(ctx context.Context, param GetVersionPatchParam, err error) (string, error) {
	changes, err := patcher.CalculateDiff(param.TargetVersionFileHashes, param.CurrentVersionFileHashes)
	if err != nil {
		l.logger.Error("Failed to calculate diff",
			zap.String("resource ID", param.ResourceID),
			zap.Int("target version ID", param.TargetVersionID),
			zap.Int("current version ID", param.CurrentVersionID),
			zap.Error(err),
		)
		return "", err
	}

	patchDir := l.storage.PatchDir(param.ResourceID, param.TargetVersionID)

	latestStorage, err := l.storageRepo.GetStorageByVersionID(ctx, param.TargetVersionID)
	if err != nil {
		l.logger.Error("Failed to get storage",
			zap.Error(err),
		)
		return "", err

	}

	patchName, err := patcher.Generate(strconv.Itoa(param.CurrentVersionID), latestStorage.Directory, patchDir, changes)
	if err != nil {
		l.logger.Error("Failed to generate patch package",
			zap.Error(err),
		)
		return "", err

	}

	return filepath.Join(patchDir, patchName), nil
}

func (l *VersionLogic) GetCacheGroup() *cache.VersionCacheGroup {
	return l.cacheGroup
}

func (l *VersionLogic) GetStorageRootDir() string {
	return l.storage.RootDir
}
