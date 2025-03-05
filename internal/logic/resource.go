package logic

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/cache"

	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/repo"

	"github.com/MirrorChyan/resource-backend/internal/ent"

	"go.uber.org/zap"
)

type ResourceLogic struct {
	logger       *zap.Logger
	resourceRepo *repo.Resource
	cg           *cache.MultiCacheGroup
}

func NewResourceLogic(
	logger *zap.Logger,
	resourceRepo *repo.Resource,
	cg *cache.MultiCacheGroup,
) *ResourceLogic {
	return &ResourceLogic{
		logger:       logger,
		resourceRepo: resourceRepo,
		cg:           cg,
	}
}

func (l *ResourceLogic) FindById(ctx context.Context, id string) (*ent.Resource, error) {
	key := l.cg.GetCacheKey(id)
	val, err := l.cg.ResourceInfoCache.ComputeIfAbsent(key, func() (*ent.Resource, error) {
		return l.resourceRepo.FindById(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	return *val, err
}

func (l *ResourceLogic) Create(ctx context.Context, param CreateResourceParam) (*ent.Resource, error) {
	return l.resourceRepo.CreateResource(ctx, param.ID, param.Name, param.Description)
}

func (l *ResourceLogic) Exists(ctx context.Context, id string) (bool, error) {
	return l.resourceRepo.CheckResourceExistsByID(ctx, id)
}
