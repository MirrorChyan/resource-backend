package repo

import (
	"context"
	"entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/pkg/cursor"
	"github.com/MirrorChyan/resource-backend/internal/pkg/sortorder"

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

func (r *Resource) ListResource(
	ctx context.Context,
	lastCursor *cursor.Cursor,
	limit int,
	order sortorder.Order,
) ([]*ent.Resource, *cursor.Cursor, bool, error) {

	var (
		hasMore bool
	)

	query := r.db.Resource.Query().
		Limit(limit + 1)

	switch order {
	case sortorder.Newest:
		query = query.Order(resource.ByCreatedAt(sql.OrderDesc()))
	case sortorder.Oldest:
		query = query.Order(resource.ByCreatedAt(sql.OrderAsc()))
	}

	if lastCursor != nil {
		switch order {
		case sortorder.Newest:
			query = query.Where(
				resource.Or(
					resource.CreatedAtLT(lastCursor.CreatedAt),
					resource.And(
						resource.CreatedAtEQ(lastCursor.CreatedAt),
						resource.IDGT(lastCursor.ID),
					)),
			)
		case sortorder.Oldest:
			query = query.Where(
				resource.Or(
					resource.CreatedAtGT(lastCursor.CreatedAt),
					resource.And(
						resource.CreatedAtEQ(lastCursor.CreatedAt),
						resource.IDGT(lastCursor.ID),
					),
				),
			)
		}
	}

	list, err := query.All(ctx)
	if err != nil {
		return nil, nil, false, err
	}

	if len(list) > limit {
		hasMore = true
		list = list[:limit]
	}

	var currentCursor *cursor.Cursor
	if len(list) > 0 {
		lastItem := list[len(list)-1]
		currentCursor = &cursor.Cursor{
			ID:        lastItem.ID,
			CreatedAt: lastItem.CreatedAt,
		}
	}

	return list, currentCursor, hasMore, nil
}
