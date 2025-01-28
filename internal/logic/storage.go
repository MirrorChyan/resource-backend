package logic

import (
	"context"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"os"
	"path/filepath"
	"strconv"

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

func (l *StorageLogic) CreateFullUpdateStorage(ctx context.Context, tx *ent.Tx, verID int, os, arch, fullUpdatePath, resourcePath string, fileHashes map[string]string) (*ent.Storage, error) {
	storage, err := l.storageRepo.CreateFullUpdateStorage(ctx, tx, verID, os, arch, fullUpdatePath, resourcePath, fileHashes)
	if err != nil {
		l.logger.Error("create full update storage failed",
			zap.Error(err),
		)
		return nil, err
	}

	return storage, nil
}

func (l *StorageLogic) CreateIncrementalUpdateStorage(ctx context.Context, tx *ent.Tx, target, current int, os, arch, incrementalUpdatePath string) (*ent.Storage, error) {
	storage, err := l.storageRepo.CreateIncrementalUpdateStorage(ctx, tx, target, current, os, arch, incrementalUpdatePath)
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
	storage, err := l.storageRepo.GetFullUpdateStorage(ctx, versionId, os, arch)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, err
		}
		l.logger.Error("get full update storage failed",
			zap.Error(err),
		)
		return nil, err
	}

	return storage, nil
}

func (l *StorageLogic) GetFullUpdatePath(ctx context.Context, versionId int, os, arch string) (string, error) {
	storage, err := l.GetFullUpdateStorage(ctx, versionId, os, arch)
	if err != nil {
		return "", err
	}

	return storage.PackagePath, nil
}

func (l *StorageLogic) GetIncrementalUpdatePath(ctx context.Context, param model.UpdateProcessInfo) (string, error) {
	storage, err := l.storageRepo.GetIncrementalUpdatePath(
		ctx,
		param.TargetVersionID, param.CurrentVersionID,
		param.OS, param.Arch,
	)
	if err != nil {
		return "", err
	}
	return storage.PackagePath, nil
}

func (l *StorageLogic) BuildVersionStorageDirPath(resID string, verID int, os, arch string) string {
	platformDir := l.getPlatformDirName(os, arch)
	verIDStr := strconv.Itoa(verID)
	return filepath.Join(l.RootDir, resID, "versions", verIDStr, platformDir)
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
