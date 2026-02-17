package repo

import (
	"context"
	"entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/pkg/sortorder"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

type Resource struct {
	*Repo
}

func NewResource(db *Repo) *Resource {
	return &Resource{
		Repo: db,
	}
}

func (r *Resource) FindUpdateTypeById(ctx context.Context, id string) (*ent.Resource, error) {
	return r.db.Resource.Query().
		Select(resource.FieldUpdateType).
		Where(resource.ID(id)).
		First(ctx)
}

func (r *Resource) CreateResource(ctx context.Context, resID, name, description, updateType string) (*ent.Resource, error) {
	return r.db.Resource.Create().
		SetID(resID).
		SetName(name).
		SetUpdateType(updateType).
		SetDescription(description).
		Save(ctx)
}

func (r *Resource) CheckResourceExistsByID(ctx context.Context, id string) (bool, error) {
	return r.db.Resource.Query().
		Where(resource.ID(id)).
		Exist(ctx)
}

func (r *Resource) GetResourceByID(ctx context.Context, id string) (*ent.Resource, error) {
	return r.db.Resource.Query().
		Where(resource.ID(id)).
		First(ctx)
}

func (r *Resource) UpdateResource(
	ctx context.Context,
	id string,
	name string,
	description string,
	updateType string,
) (*ent.Resource, error) {
	return r.db.Resource.UpdateOneID(id).
		SetName(name).
		SetDescription(description).
		SetUpdateType(updateType).
		Save(ctx)
}

func (r *Resource) DeleteResourceByID(ctx context.Context, id string) error {
	return r.db.Resource.DeleteOneID(id).Exec(ctx)
}

func (r *Resource) ForceDeleteResourceByID(ctx context.Context, id string) error {
	return r.WithTx(ctx, func(tx *ent.Tx) error {
		_, err := tx.Storage.Delete().
			Where(storage.HasVersionWith(version.HasResourceWith(resource.ID(id)))).
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.Version.Delete().
			Where(version.HasResourceWith(resource.ID(id))).
			Exec(ctx)
		if err != nil {
			return err
		}

		return tx.Resource.DeleteOneID(id).Exec(ctx)
	})
}

func (r *Resource) HasVersions(ctx context.Context, id string) (bool, error) {
	return r.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(id))).
		Exist(ctx)
}

func (r *Resource) GetFullResource(ctx context.Context) ([]*ent.Resource, error) {
	return r.db.Resource.Query().All(ctx)
}

func (r *Resource) ListResource(
	ctx context.Context,
	offset int,
	limit int,
	order sortorder.Order,
) ([]*ent.Resource, int, bool, error) {

	var (
		hasMore bool
	)

	total, err := r.db.Resource.Query().Count(ctx)
	if err != nil {
		return nil, 0, false, err
	}

	query := r.db.Resource.Query().
		Offset(offset).
		Limit(limit + 1)

	switch order {
	case sortorder.Newest:
		query = query.Order(resource.ByCreatedAt(sql.OrderDesc()))
	case sortorder.Oldest:
		query = query.Order(resource.ByCreatedAt(sql.OrderAsc()))
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
