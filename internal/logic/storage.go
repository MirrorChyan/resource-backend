package logic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/MirrorChyan/resource-backend/internal/ent/version"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"go.uber.org/zap"
)

type StorageLogic struct {
	logger      *zap.Logger
	storageRepo *repo.Storage
	RootDir     string
}

func NewStorageLogic(logger *zap.Logger, storageRepo *repo.Storage) *StorageLogic {
	// change to configurable, this is only a temporary solution
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	rootDir := filepath.Join(dir, "storage")
	return &StorageLogic{
		logger:      logger,
		storageRepo: storageRepo,
		RootDir:     rootDir,
	}
}

func (l *StorageLogic) CreateFullUpdateStorage(ctx context.Context, tx *ent.Tx, verID int, os, arch, fullUpdatePath, packageSHA256, resourcePath string, fileHashes map[string]string) (*ent.Storage, error) {
	storage, err := l.storageRepo.CreateFullUpdateStorage(ctx, tx, verID, os, arch, fullUpdatePath, packageSHA256, resourcePath, fileHashes)
	if err != nil {
		l.logger.Error("create full update storage failed",
			zap.Error(err),
		)
		return nil, err
	}

	return storage, nil
}

func (l *StorageLogic) CreateIncrementalUpdateStorage(ctx context.Context, tx *ent.Tx, target, current int, os, arch, incrementalUpdatePath, packageSHA256 string) (*ent.Storage, error) {
	storage, err := l.storageRepo.CreateIncrementalUpdateStorage(ctx, tx, target, current, os, arch, incrementalUpdatePath, packageSHA256)
	if err != nil {
		l.logger.Error("create incremental update storage failed",
			zap.Error(err),
		)
		return nil, err
	}

	return storage, nil
}

func (l *StorageLogic) CheckStorageExist(ctx context.Context, verID int, os, arch string) (bool, error) {
	exist, err := l.storageRepo.CheckStorageExist(ctx, verID, os, arch)
	if err != nil {
		l.logger.Error("check storage exist failed",
			zap.Error(err),
		)
		return false, err
	}

	return exist, nil
}

func (l *StorageLogic) GetFullUpdateStorage(ctx context.Context, versionId int, os, arch string) (*ent.Storage, error) {
	return l.storageRepo.GetFullUpdateStorage(ctx, versionId, os, arch)
}

func (l *StorageLogic) GetIncrementalUpdateStorage(ctx context.Context, targerVerID, currentVerID int, os, arch string) (*ent.Storage, error) {
	return l.storageRepo.GetIncrementalUpdateStorage(ctx, targerVerID, currentVerID, os, arch)
}

func (l *StorageLogic) BuildVersionStorageDirPath(resID string, verID int, os, arch string) string {
	platformDir := l.getPlatformDirName(os, arch)
	verIDStr := strconv.Itoa(verID)
	return filepath.Join(l.RootDir, resID, verIDStr, platformDir)
}

func (l *StorageLogic) getPlatformDirName(os, arch string) string {
	if os == "" && arch == "" {
		return "any"
	}
	if os == "" {
		return fmt.Sprintf("any-%s", arch)
	}
	if arch == "" {
		return fmt.Sprintf("%s-any", os)
	}
	return fmt.Sprintf("%s-%s", os, arch)
}

func (l *StorageLogic) BuildVersionResourceStorageDirPath(resID string, verID int, os, arch string) string {
	return filepath.Join(l.BuildVersionStorageDirPath(resID, verID, os, arch), "resource")
}

func (l *StorageLogic) BuildVersionResourceStoragePath(resID string, verID int, os, arch string) string {
	return filepath.Join(l.BuildVersionStorageDirPath(resID, verID, os, arch), "resource.zip")
}

func (l *StorageLogic) BuildVersionPatchStorageDirPath(resID string, verID int, os, arch string) string {
	return filepath.Join(l.BuildVersionStorageDirPath(resID, verID, os, arch), "patch")
}

func (l *StorageLogic) BuildVersionPatchStoragePath(resID string, verID, oldVerID int, os, arch string) string {
	patchName := fmt.Sprintf("%d.zip", oldVerID)
	return filepath.Join(l.BuildVersionPatchStorageDirPath(resID, verID, os, arch), patchName)
}

func (l *StorageLogic) ClearOldStorages(ctx context.Context, resID string, channel version.Channel, latestVerID int) error {
	// get all old full update storages
	fullUpdateStorages, err := l.storageRepo.GetOldFullUpdateStorages(ctx, resID, channel, latestVerID)
	if err != nil {
		l.logger.Error("get old version full update storages failed",
			zap.String("resource id", resID),
			zap.String("channel", channel.String()),
			zap.Int("latest version id", latestVerID),
			zap.Error(err),
		)
		return err
	}

	// delete old full update package
	for _, storage := range fullUpdateStorages {
		if storage.PackagePath != "" {
			err = os.Remove(storage.PackagePath)
			if err != nil && !os.IsNotExist(err) {
				l.logger.Error("delete old version full update package failed",
					zap.String("package path", storage.PackagePath),
					zap.Error(err),
				)
				return err
			}
		}

		if storage.ResourcePath == "" {
			continue
		}

		if err = os.RemoveAll(storage.ResourcePath); err != nil {
			l.logger.Error("delete old version full update resource failed",
				zap.String("resource path", storage.ResourcePath),
				zap.Error(err),
			)
			return err
		}
	}

	// clear old full update storages
	err = l.storageRepo.ClearOldFullUpdateStorages(ctx, resID, channel, latestVerID)
	if err != nil {
		l.logger.Error("clear old version full update storages failed",
			zap.String("resource id", resID),
			zap.String("channel", channel.String()),
			zap.Int("latest version id", latestVerID),
			zap.Error(err),
		)
		return err
	}

	// get all old incremental update storages
	incrementalUpdateStorages, err := l.storageRepo.GetOldIncrementalUpdateStorages(ctx, resID, channel, latestVerID)
	if err != nil {
		l.logger.Error("get old version incremental update storages failed",
			zap.String("resource id", resID),
			zap.String("channel", channel.String()),
			zap.Int("latest version id", latestVerID),
			zap.Error(err),
		)
		return err
	}

	// delete old incremental update package
	for _, storage := range incrementalUpdateStorages {
		err = os.Remove(storage.PackagePath)
		if err != nil && !os.IsNotExist(err) {
			l.logger.Error("delete old version incremental update package failed",
				zap.String("package path", storage.PackagePath),
				zap.Error(err),
			)
			return err
		}
	}

	// delete old incremental update storages
	err = l.storageRepo.DeleteOldIncrementalUpdateStorages(ctx, resID, channel, latestVerID)
	if err != nil {
		l.logger.Error("delete old version incremental update storages failed",
			zap.String("resource id", resID),
			zap.String("channel", channel.String()),
			zap.Int("latest version id", latestVerID),
			zap.Error(err),
		)
		return err
	}

	return nil
}

func (l *StorageLogic) SetPackageSHA256(ctx context.Context, storageID int, sha256 string) error {
	return l.storageRepo.SetPackageHashSHA256(ctx, storageID, sha256)
}
