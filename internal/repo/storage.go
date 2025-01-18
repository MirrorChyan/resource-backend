package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

type Storage struct {
	db *ent.Client
}

func NewStorage(db *ent.Client) *Storage {
	return &Storage{
		db: db,
	}
}

func (r *Storage) CreateStorage(ctx context.Context, tx *ent.Tx, dir string) (*ent.Storage, error) {
	return tx.Storage.Create().
		SetDirectory(dir).
		Save(ctx)
}

func (r *Storage) GetStorageByVersionID(ctx context.Context, verID int) (*ent.Storage, error) {
	return r.db.Storage.Query().
		Where(storage.HasVersionWith(version.ID(verID))).
		Only(ctx)
}
