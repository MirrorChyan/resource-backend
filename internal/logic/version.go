package logic

import (
	"context"
	"fmt"

	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/patcher"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"github.com/MirrorChyan/resource-backend/internal/pkg/stg"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/ksuid"

	"go.uber.org/zap"
)

type VersionLogic struct {
	logger               *zap.Logger
	versionRepo          *repo.Version
	storageRepo          *repo.Storage
	tempDownloadInfoRepo *repo.TempDownloadInfo
	storage              *stg.Storage
}

func NewVersionLogic(
	logger *zap.Logger,
	versionRepo *repo.Version,
	storageRepo *repo.Storage,
	tempDownloadInfoRepo *repo.TempDownloadInfo,
	storage *stg.Storage,
) *VersionLogic {
	return &VersionLogic{
		logger:               logger,
		versionRepo:          versionRepo,
		storageRepo:          storageRepo,
		tempDownloadInfoRepo: tempDownloadInfoRepo,
		storage:              storage,
	}
}

func (l *VersionLogic) NameExists(ctx context.Context, param VersionNameExistsParam) (bool, error) {
	return l.versionRepo.CheckVersionExistsByName(ctx, param.ResourceID, param.Name)
}

func (l *VersionLogic) GetLatest(ctx context.Context, resourceID string) (*ent.Version, error) {
	return l.versionRepo.GetLatestVersion(ctx, resourceID)
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, error) {
	var number uint64
	latest, err := l.versionRepo.GetLatestVersion(ctx, param.ResourceID)
	if ent.IsNotFound(err) {
		number = 1
	} else if err != nil {
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
		saveDir := filepath.Join(versionDir, "resource")
		if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
			l.logger.Error("Failed to create storage directory",
				zap.String("directory", saveDir),
				zap.Error(err),
			)
			return err
		}

		if strings.HasSuffix(param.UploadArchivePath, ".zip") {
			err = archive.UnpackZip(param.UploadArchivePath, saveDir)
		} else if strings.HasSuffix(param.UploadArchivePath, ".tar.gz") {
			err = archive.UnpackTarGz(param.UploadArchivePath, saveDir)
		} else {
			l.logger.Error("Unknown archive extension",
				zap.String("archive path", param.UploadArchivePath),
			)
			return errors.New("unknown archive extension")
		}

		tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
			return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
				// Code before the actual rollback.

				rmErr := os.RemoveAll(saveDir)
				if rmErr != nil {
					l.logger.Error("Failed to remove storage directory",
						zap.Error(rmErr),
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

		archivePath := filepath.Join(versionDir, "resource.zip")
		if strings.HasSuffix(param.UploadArchivePath, ".zip") {
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
			if err := os.Remove(param.UploadArchivePath); err != nil {
				l.logger.Error("Failed to remove temp file",
					zap.Error(err),
				)
				return err
			}
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

		stg, err := l.storageRepo.CreateStorage(ctx, tx, saveDir)
		if err != nil {
			l.logger.Error("Failed to create storage",
				zap.Error(err),
			)
			return err
		}

		ver, err = l.versionRepo.SetVersionStorageByOne(ctx, tx, ver, stg)
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

	return ver, nil
}

func (l *VersionLogic) GetByName(ctx context.Context, param GetVersionByNameParam) (*ent.Version, error) {
	return l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.Name)
}

func (l *VersionLogic) StoreTempDownloadInfo(ctx context.Context, param StoreTempDownloadInfoParam) (string, error) {
	isFull := param.CurrentVersionName == ""

	// if current version is not provided, we will download the full version
	var (
		current *ent.Version
		err     error
	)
	if !isFull {
		getVersionByNameParam := GetVersionByNameParam{
			ResourceID: param.ResourceID,
			Name:       param.CurrentVersionName,
		}
		current, err = l.GetByName(ctx, getVersionByNameParam)
		if err != nil {
			if !ent.IsNotFound(err) {
				l.logger.Error("Failed to get current version",
					zap.Error(err),
				)
				return "", err
			}
			isFull = true
		}
	}

	var info = &TempDownloadInfo{
		ResourceID:      param.ResourceID,
		Full:            isFull,
		TargetVersionID: param.LatestVersion.ID,
	}

	if !isFull {
		info.TargetVersionFileHashes = param.LatestVersion.FileHashes
		info.CurrentVersionID = current.ID
		info.CurrentVersionFileHashes = current.FileHashes
	}

	key := ksuid.New().String()
	rk := fmt.Sprintf("RES:%v", key)

	err = l.tempDownloadInfoRepo.SetTempDownloadInfo(ctx, rk, info, 10*time.Minute)
	if err != nil {
		l.logger.Error("Failed to set temp download info",
			zap.Error(err),
		)
		return "", err
	}

	return key, nil
}

func (l *VersionLogic) GetTempDownloadInfo(ctx context.Context, key string) (*TempDownloadInfo, error) {
	rk := fmt.Sprintf("RES:%v", key)

	info, err := l.tempDownloadInfoRepo.GetDelTempDownloadInfo(ctx, rk)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			l.logger.Error("redis err failed to get temp download info",
				zap.Error(err),
			)
		}
		return nil, err
	}

	return info, nil
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

	patchPath := filepath.Join(patchDir, patchName)
	return patchPath, nil
}
