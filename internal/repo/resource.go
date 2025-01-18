package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
)

type Resource struct {
	db *ent.Client
}

func NewResource(db *ent.Client) *Resource {
	return &Resource{
		db: db,
	}
}

func (r *Resource) CreateResource(ctx context.Context, resID, name, description string) (*ent.Resource, error) {
	return r.db.Resource.Create().
		SetID(resID).
		SetName(name).
		SetDescription(description).
		Save(ctx)
}

func (r *Resource) CheckResourceExistsByID(ctx context.Context, id string) (bool, error) {
	return r.db.Resource.Query().
		Where(resource.ID(id)).
		Exist(ctx)
}
