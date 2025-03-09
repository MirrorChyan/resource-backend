package logic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/bytedance/sonic"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"go.uber.org/zap"
)

type StorageLogic struct {
	logger       *zap.Logger
	storageRepo  *repo.Storage
	resourceRepo *repo.Resource
	rawQuery     *repo.RawQuery
	RootDir      string
	OSSDir       string
}

func NewStorageLogic(
	logger *zap.Logger,
	storageRepo *repo.Storage,
	resourceRepo *repo.Resource,
	rawQuery *repo.RawQuery,
) *StorageLogic {
	// change to configurable, this is only a temporary solution
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return &StorageLogic{
		logger:       logger,
		resourceRepo: resourceRepo,
		storageRepo:  storageRepo,
		rawQuery:     rawQuery,
		RootDir:      filepath.Join(dir, "storage"),
		OSSDir:       filepath.Join(dir, "oss"),
	}
}

func (l *StorageLogic) CreateFullUpdateStorage(ctx context.Context, verID int, os, arch, fullUpdatePath, packageSHA256 string, fileHashes map[string]string) (*ent.Storage, error) {
	storage, err := l.storageRepo.CreateFullUpdateStorage(ctx, verID, os, arch, fullUpdatePath, packageSHA256, fileHashes)
	if err != nil {
		l.logger.Error("create full update storage failed",
			zap.Error(err),
		)
		return nil, err
	}

	return storage, nil
}

func (l *StorageLogic) CreateIncrementalUpdateStorage(ctx context.Context, tx *ent.Tx, target, current int, os, arch, path, hashes string) (*ent.Storage, error) {
	storage, err := l.storageRepo.CreateIncrementalUpdateStorage(ctx, tx, target, current, os, arch, path, hashes)
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

func (l *StorageLogic) BuildVersionResourceStoragePath(resID string, verID int, os, arch, filename string) string {
	return filepath.Join(l.BuildVersionStorageDirPath(resID, verID, os, arch), filename)
}

func (l *StorageLogic) BuildVersionPatchStorageDirPath(resID string, verID int, os, arch string) string {
	return filepath.Join(l.BuildVersionStorageDirPath(resID, verID, os, arch), "patch")
}

func (l *StorageLogic) BuildVersionPatchStoragePath(resID string, verID, oldVerID int, os, arch string) string {
	patchName := fmt.Sprintf("%d.zip", oldVerID)
	return filepath.Join(l.BuildVersionPatchStorageDirPath(resID, verID, os, arch), patchName)
}

func (l *StorageLogic) UpdateStoragePackageHash(ctx context.Context, id int, hash string) error {
	return l.storageRepo.UpdateStoragePackageHash(ctx, id, hash)
}

func (l *StorageLogic) ClearOldStorages(ctx context.Context) error {
	resource, err := l.resourceRepo.GetFullResource(ctx)
	if err != nil {
		l.logger.Error("failed to get resource",
			zap.Error(err),
		)
		return err
	}
	for _, val := range resource {
		if err := l.doPurgeResource(ctx, val.ID); len(err) > 0 {
			je := errors.Join(err...)
			l.logger.Error("failed to purge resource",
				zap.String("resource id", val.ID),
				zap.Error(je),
			)
			go doErrorNotify(l.logger, je.Error())
		}
	}
	return nil
}

func (l *StorageLogic) doPurgeResource(ctx context.Context, resourceId string) []error {
	info, err := l.rawQuery.GetReadyToPurgeInfo(resourceId)
	var el []error
	switch {
	case err != nil:
		return append(el, err)
	case len(info) == 0:
		return nil
	}

	for _, val := range info {
		var (
			key = filepath.Join(val.ResourceId, strconv.Itoa(val.VersionId), l.getPlatformDirName(val.OS, val.Arch))
			od  = filepath.Join(l.OSSDir, key)
			ld  = filepath.Join(l.RootDir, key)
		)
		l.logger.Info("clear old storage",
			zap.String("oss dir", od),
			zap.String("local dir", ld),
		)
		if err := os.RemoveAll(od); err != nil {
			l.logger.Error("failed to remove old storage",
				zap.String("oss dir", od),
				zap.Error(err),
			)
			el = append(el, err)
		}
		if err := os.RemoveAll(ld); err != nil {
			l.logger.Error("failed to remove local storage",
				zap.String("local dir", ld),
				zap.Error(err),
			)
			el = append(el, err)
		}

		err := l.storageRepo.PurgeStorageInfo(ctx, val.StorageId)
		if err != nil {
			l.logger.Error("failed to purge storage info",
				zap.Int("storage id", val.StorageId),
				zap.Error(err),
			)
			return append(el, err)
		}
	}
	return el
}

func doErrorNotify(l *zap.Logger, msg string) {
	var (
		cfg = config.GConfig
	)
	webhook := cfg.Extra.PurgeErrorWebhook
	if webhook == "" {
		return
	}
	buf, e := sonic.Marshal(map[string]string{
		"msg": msg,
	})
	if e != nil {
		l.Warn("Failed to marshal CreateNewVersion callback")
		return
	}
	_, err := http.Post(webhook, "application/json", bytes.NewBuffer(buf))
	if err != nil {
		l.Warn("Failed to send CreateNewVersion callback")
	}
}
