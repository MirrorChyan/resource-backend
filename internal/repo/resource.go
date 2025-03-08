package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
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

func (r *Resource) GetFullResource(ctx context.Context) ([]*ent.Resource, error) {
	return r.db.Resource.Query().All(ctx)
}
