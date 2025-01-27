package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

type Version struct {
	db *ent.Client
}

func NewVersion(db *ent.Client) *Version {
	return &Version{
		db: db,
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

func (r *Version) CreateVersion(ctx context.Context, resID, name string, number uint64) (*ent.Version, error) {
	return r.db.Version.Create().
		SetResourceID(resID).
		SetName(name).
		SetNumber(number).
		Save(ctx)
}
