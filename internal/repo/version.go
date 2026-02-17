package repo

import (
	"context"
	"entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/pkg/sortorder"

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

func (r *Version) GetVersionByID(ctx context.Context, resID string, verID int) (*ent.Version, error) {
	return r.db.Version.Query().
		Where(version.ID(verID)).
		Where(version.HasResourceWith(resource.ID(resID))).
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

func (r *Version) ListVersion(
	ctx context.Context,
	resourceID string,
	offset int,
	limit int,
	order sortorder.Order,
) ([]*ent.Version, int, bool, error) {
	var hasMore bool

	query := r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resourceID)))

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, false, err
	}

	query = r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resourceID))).
		Offset(offset).
		Limit(limit + 1)

	switch order {
	case sortorder.Newest:
		query = query.Order(version.ByCreatedAt(sql.OrderDesc()))
	case sortorder.Oldest:
		query = query.Order(version.ByCreatedAt(sql.OrderAsc()))
	}

	list, err := query.All(ctx)
	if err != nil {
		return nil, 0, false, err
	}

	if len(list) > limit {
		hasMore = true
		list = list[:limit]
	}

	return list, total, hasMore, nil
}
