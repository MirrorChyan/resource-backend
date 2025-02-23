package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

type Version struct {
	*Repo
}

func NewVersion(db *Repo) *Version {
	return &Version{
		Repo: db,
	}
}

func (r *Version) GetVersionByName(ctx context.Context, resID, name string) (*ent.Version, error) {
	return r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resID))).
		Where(version.Name(name)).
		First(ctx)
}

func (r *Version) GetMaxNumberVersion(ctx context.Context, resID string) (*ent.Version, error) {
	return r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resID))).
		Order(ent.Desc(version.FieldNumber)).
		First(ctx)
}

func (r *Version) CreateVersion(ctx context.Context, resID string, channel version.Channel, name string, number uint64) (*ent.Version, error) {
	return r.db.Version.Create().
		SetResourceID(resID).
		SetChannel(channel).
		SetName(name).
		SetNumber(number).
		Save(ctx)
}

func (r *Version) UpdateVersionReleaseNote(ctx context.Context, verID int, releaseNote string) error {
	return r.db.Version.UpdateOneID(verID).
		SetReleaseNote(releaseNote).
		Exec(ctx)
}

func (r *Version) UpdateVersionCustomData(ctx context.Context, verID int, customData string) error {
	return r.db.Version.UpdateOneID(verID).
		SetCustomData(customData).
		Exec(ctx)
}
