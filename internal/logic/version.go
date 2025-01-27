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
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/ksuid"

	"go.uber.org/zap"
)

type VersionLogic struct {
	logger               *zap.Logger
	storageLogic         *StorageLogic
	repo                 *repo.Repo
	versionRepo          *repo.Version
	tempDownloadInfoRepo *repo.TempDownloadInfo
}

func NewVersionLogic(
	logger *zap.Logger,
	storageLogic *StorageLogic,
	repo *repo.Repo,
	versionRepo *repo.Version,
	tempDownloadInfoRepo *repo.TempDownloadInfo,
) *VersionLogic {
	return &VersionLogic{
		logger:               logger,
		storageLogic:         storageLogic,
		repo:                 repo,
		versionRepo:          versionRepo,
		tempDownloadInfoRepo: tempDownloadInfoRepo,
	}
}

func (l *VersionLogic) NameExists(ctx context.Context, param VersionNameExistsParam) (bool, error) {
	ver, err := l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.Name)
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

func (l *VersionLogic) GetLatest(ctx context.Context, resourceID string) (*ent.Version, error) {
	return l.versionRepo.GetLatestVersion(ctx, resourceID)
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, error) {
	ver, err := l.versionRepo.GetVersionByName(ctx, param.ResourceID, param.Name)
	if err != nil && ent.IsNotFound(err) {
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

		ver, err = l.versionRepo.CreateVersion(ctx, param.ResourceID, param.Name, number)
		if err != nil {
			l.logger.Error("Failed to create version",
				zap.String("resource id", param.ResourceID),
				zap.String("version name", param.Name),
				zap.Error(err),
			)
			return nil, err
		}
	} else if err != nil {
		l.logger.Error("Failed to get version",
			zap.String("resource id", param.ResourceID),
			zap.String("version name", param.Name),
		)
		return nil, err
	}

	err = l.repo.WithTx(ctx, func(tx *ent.Tx) error {
		saveDir := l.storageLogic.BuildVersionResourceStorageDirPath(param.ResourceID, ver.ID, param.OS, param.Arch)
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

		archivePath := l.storageLogic.BuildVersionResourceStoragePath(param.ResourceID, ver.ID, param.OS, param.Arch)
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

		_, err = l.storageLogic.CreateFullUpdateStorage(ctx, tx, ver.ID, param.OS, param.Arch, archivePath, saveDir, fileHashes)
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
		OS:              param.OS,
		Arch:            param.Arch,
	}

	if !isFull {
		info.CurrentVersionID = current.ID
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

func (l *VersionLogic) GetFullUpdatePackagePath(ctx context.Context, param GetFullUpdatePackagePathParam) (string, error) {
	return l.storageLogic.GetFullUpdatePath(ctx, param.VersionID, param.OS, param.Arch)
}

func (l *VersionLogic) CreateIncrementalUpdatePackage(ctx context.Context, resID string, verID, oldID int, resOS, resArch string) (string, error) {
	var packagePath string
	targetStorage, err := l.storageLogic.GetFullUpdateStorage(ctx, verID, resOS, resArch)
	if err != nil {
		l.logger.Error("Failed to get target full update storage",
			zap.String("resource id", resID),
			zap.Int("version id", verID),
			zap.String("resource os", resOS),
			zap.String("resource arch", resArch),
			zap.Error(err),
		)
		return "", err
	}

	currentStorage, err := l.storageLogic.GetFullUpdateStorage(ctx, oldID, resOS, resArch)
	if err != nil {
		l.logger.Error("Failed to get current full update storage",
			zap.String("resource id", resID),
			zap.Int("version id", oldID),
			zap.String("resource os", resOS),
			zap.String("resource arch", resArch),
			zap.Error(err),
		)
		return "", err
	}

	changes, err := patcher.CalculateDiff(targetStorage.FileHashes, currentStorage.FileHashes)
	if err != nil {
		l.logger.Error("Failed to calculate diff",
			zap.String("resource ID", resID),
			zap.Int("target version ID", verID),
			zap.Int("current version ID", oldID),
			zap.Error(err),
		)
		return "", err
	}

	patchDir := l.storageLogic.BuildVersionPatchStorageDirPath(resID, verID, resOS, resArch)

	resourceDir := targetStorage.ResourcePath

	patchName, err := patcher.Generate(strconv.Itoa(oldID), resourceDir, patchDir, changes)
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

				rmErr := os.RemoveAll(packagePath)
				if rmErr != nil {
					l.logger.Error("Failed to remove patch package",
						zap.Error(rmErr),
					)
				}

				err := next.Rollback(ctx, tx)
				// Code after the transaction was rolled back.

				return err
			})
		})

		packagePath = filepath.Join(patchDir, patchName)
		_, err = l.storageLogic.CreateIncrementalUpdateStorage(ctx, tx, verID, oldID, resOS, resArch, packagePath)
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

func (l *VersionLogic) GetIncrementalUpdatePackagePath(ctx context.Context, param GetIncrementalUpdatePackagePathParam) (string, error) {
	packagePath, err := l.storageLogic.GetIncrementalUpdatePath(ctx, param.VersionID, param.OldVersionID, param.OS, param.Arch)
	if err != nil && ent.IsNotFound(err) {

		packagePath, err = l.CreateIncrementalUpdatePackage(ctx, param.ResourceID, param.VersionID, param.OldVersionID, param.OS, param.Arch)
		if err != nil {
			l.logger.Error("Failed to generate incremental update package",
				zap.Error(err),
			)
			return "", err
		}

	} else if err != nil {

		l.logger.Error("Failed to get incremental update package path",
			zap.Error(err),
		)
		return "", err

	}

	return packagePath, nil
}
