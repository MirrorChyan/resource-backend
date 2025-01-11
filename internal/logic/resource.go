package logic

import (
	"context"
	. "github.com/MirrorChyan/resource-backend/internal/model"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"

	"go.uber.org/zap"
)

type ResourceLogic struct {
	logger *zap.Logger
	db     *ent.Client
}

func NewResourceLogic(logger *zap.Logger, db *ent.Client) *ResourceLogic {
	return &ResourceLogic{
		logger: logger,
		db:     db,
	}
}

func (l *ResourceLogic) Create(ctx context.Context, param CreateResourceParam) (*ent.Resource, error) {
	return l.db.Resource.Create().
		SetName(param.Name).
		SetDescription(param.Description).
		Save(ctx)
}

func (l *ResourceLogic) Exists(ctx context.Context, id int) (bool, error) {
	return l.db.Resource.Query().
		Where(resource.ID(id)).
		Exist(ctx)
}

func (l *ResourceLogic) GetByID(ctx context.Context, id int) (*ent.Resource, error) {
	return l.db.Resource.Get(ctx, id)
}

func (l *ResourceLogic) List(ctx context.Context, param ListVersionParam) (int, []*ent.Resource, error) {
	query := l.db.Resource.Query()

	count, err := query.Count(ctx)
	if err != nil {
		return 0, nil, err
	}

	resources, err := query.
		Offset(param.Offset).
		Limit(param.Limit).
		All(ctx)
	if err != nil {
		return 0, nil, err
	}

	return count, resources, nil
}

func (l *ResourceLogic) Update(ctx context.Context, param UpdateResourceParam) (*ent.Resource, error) {
	return l.db.Resource.UpdateOneID(param.ID).
		SetName(param.Name).
		SetDescription(param.Description).
		SetLatestVersion(param.LatestVersion).
		Save(ctx)
}

func (l *ResourceLogic) Delete(ctx context.Context, id int) error {
	return l.db.Resource.
		DeleteOneID(id).
		Exec(ctx)
}
