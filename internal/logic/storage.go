package logic

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"go.uber.org/zap"
)

type StorageLogic struct {
	logger *zap.Logger
	db     *ent.Client
}

func NewStorageLogic(logger *zap.Logger, db *ent.Client) *StorageLogic {
	return &StorageLogic{
		logger: logger,
		db:     db,
	}
}

func (l *StorageLogic) Create(ctx context.Context, param CreateStorageParam) (*ent.Storage, error) {
	return l.db.Storage.Create().
		SetVersionID(param.VersionID).
		SetDirectory(param.Directory).
		Save(ctx)
}

func (l *StorageLogic) GetByVersionID(ctx context.Context, versionID int) (*ent.Storage, error) {
	return l.db.Storage.Query().
		Where(storage.HasVersionWith(version.ID(versionID))).
		Only(ctx)
}
