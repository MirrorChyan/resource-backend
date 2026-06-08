package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

// Read-only queries backing the admin resource-management endpoints.

func (r *Resource) buildResourceQuery(idLike, nameLike string) *ent.ResourceQuery {
	q := r.db.Resource.Query()
	if idLike != "" {
		q = q.Where(resource.IDContainsFold(idLike))
	}
	if nameLike != "" {
		q = q.Where(resource.NameContainsFold(nameLike))
	}
	return q
}

// ListResources returns resources filtered by id/name (case-insensitive contains),
// ordered by created_at desc, paginated by offset/limit.
func (r *Resource) ListResources(ctx context.Context, offset, limit int, idLike, nameLike string) ([]*ent.Resource, error) {
	return r.buildResourceQuery(idLike, nameLike).
		Order(ent.Desc(resource.FieldCreatedAt)).
		Offset(offset).
		Limit(limit).
		All(ctx)
}

func (r *Resource) CountResources(ctx context.Context, idLike, nameLike string) (int, error) {
	return r.buildResourceQuery(idLike, nameLike).Count(ctx)
}

func (r *Resource) GetResourceByID(ctx context.Context, id string) (*ent.Resource, error) {
	return r.db.Resource.Get(ctx, id)
}

func (r *Resource) CountVersions(ctx context.Context, rid string) (int, error) {
	return r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(rid))).
		Count(ctx)
}

func (r *Version) buildVersionQuery(resID, channel string) *ent.VersionQuery {
	q := r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resID)))
	if channel != "" {
		q = q.Where(version.ChannelEQ(version.Channel(channel)))
	}
	return q
}

// ListVersionsByResource returns versions of a resource, optionally filtered by
// channel, ordered by number desc, paginated by offset/limit.
func (r *Version) ListVersionsByResource(ctx context.Context, resID string, offset, limit int, channel string) ([]*ent.Version, error) {
	return r.buildVersionQuery(resID, channel).
		Order(ent.Desc(version.FieldNumber)).
		Offset(offset).
		Limit(limit).
		All(ctx)
}

func (r *Version) CountVersionsByResource(ctx context.Context, resID, channel string) (int, error) {
	return r.buildVersionQuery(resID, channel).Count(ctx)
}
