package logic

import (
	"context"

	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/repo"

	"github.com/MirrorChyan/resource-backend/internal/ent"

	"go.uber.org/zap"
)

type ResourceLogic struct {
	logger       *zap.Logger
	resourceRepo *repo.Resource
}

func NewResourceLogic(logger *zap.Logger, resourceRepo *repo.Resource) *ResourceLogic {
	return &ResourceLogic{
		logger:       logger,
		resourceRepo: resourceRepo,
	}
}

func (l *ResourceLogic) Create(ctx context.Context, param CreateResourceParam) (*ent.Resource, error) {
	return l.resourceRepo.CreateResource(ctx, param.ID, param.Name, param.Description)
}

func (l *ResourceLogic) Exists(ctx context.Context, id string) (bool, error) {
	return l.resourceRepo.CheckResourceExistsByID(ctx, id)
}
