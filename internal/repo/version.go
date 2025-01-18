package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

type Version struct {
	*Repo
	db *ent.Client
}

func NewVersion(db *ent.Client) *Version {
	return &Version{
		Repo: &Repo{db: db},
		db:   db,
	}
}

func (r *Version) CheckVersionExistsByName(ctx context.Context, resID, name string) (bool, error) {
	return r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resID))).
		Where(version.Name(name)).
		Exist(ctx)
}

func (r *Version) GetVersionByName(ctx context.Context, resID, name string) (*ent.Version, error) {
	return r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resID))).
		Where(version.Name(name)).
		First(ctx)
}

func (r *Version) GetLatestVersion(ctx context.Context, resID string) (*ent.Version, error) {
	return r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resID))).
		Order(ent.Desc(version.FieldNumber)).
		First(ctx)
}

func (r *Version) CreateVersion(ctx context.Context, tx *ent.Tx, resID, name string, number uint64) (*ent.Version, error) {
	return tx.Version.Create().
		SetResourceID(resID).
		SetName(name).
		SetNumber(number).
		Save(ctx)
}

func (r *Version) SetVersionFileHashesByOne(ctx context.Context, tx *ent.Tx, ver *ent.Version, fileHashes map[string]string) (*ent.Version, error) {
	return tx.Version.UpdateOne(ver).
		SetFileHashes(fileHashes).
		Save(ctx)
}

func (r *Version) SetVersionStorageByOne(ctx context.Context, tx *ent.Tx, ver *ent.Version, stg *ent.Storage) (*ent.Version, error) {
	return tx.Version.UpdateOne(ver).
		SetStorage(stg).
		Save(ctx)
}
