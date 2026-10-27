package logic

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"

	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/repo"

	"github.com/MirrorChyan/resource-backend/internal/ent"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type ResourceLogic struct {
	logger       *zap.Logger
	resourceRepo *repo.Resource
	platformRepo *repo.PlatformConfig
	storageRepo  *repo.Storage
	rdb          *redis.Client
	cg           *cache.MultiCacheGroup
}

func NewResourceLogic(
	logger *zap.Logger,
	resourceRepo *repo.Resource,
	platformRepo *repo.PlatformConfig,
	storageRepo *repo.Storage,
	rdb *redis.Client,
	cg *cache.MultiCacheGroup,
) *ResourceLogic {
	return &ResourceLogic{
		logger:       logger,
		resourceRepo: resourceRepo,
		platformRepo: platformRepo,
		storageRepo:  storageRepo,
		rdb:          rdb,
		cg:           cg,
	}
}

// findUpdateTypeById returns the resource's own update type; serving paths resolve the
// platform's through ResolveUpdateType instead.
func (l *ResourceLogic) findUpdateTypeById(ctx context.Context, id string) (types.Update, error) {
	key := l.cg.GetCacheKey(id)
	val, err := l.cg.ResourceInfoCache.ComputeIfAbsent(key, func() (*ent.Resource, error) {
		return l.resourceRepo.FindUpdateTypeById(ctx, id)
	})
	if err != nil {
		return "", err
	}
	return types.Update((*val).UpdateType), err
}

// ResolveUpdateType returns the update type of the platform: its own when it has one,
// the resource's otherwise.
func (l *ResourceLogic) ResolveUpdateType(ctx context.Context, id, os, arch string) (types.Update, error) {
	own, err := l.platformUpdateTypes(ctx, id)
	if err != nil {
		return "", err
	}
	if t, ok := own[Platform{OS: os, Arch: arch}]; ok {
		return t, nil
	}
	return l.findUpdateTypeById(ctx, id)
}

// platformUpdateTypes returns the update types of the platforms of the resource that
// have their own; the map is shared through the cache, never modify it.
func (l *ResourceLogic) platformUpdateTypes(ctx context.Context, id string) (map[Platform]types.Update, error) {
	key := l.cg.GetCacheKey(id)
	val, err := l.cg.PlatformUpdateTypeCache.ComputeIfAbsent(key, func() (map[Platform]types.Update, error) {
		rows, err := l.platformRepo.ListByResource(ctx, id)
		if err != nil {
			return nil, err
		}
		own := make(map[Platform]types.Update, len(rows))
		for _, row := range rows {
			own[Platform{OS: row.Os, Arch: row.Arch}] = types.Update(row.UpdateType)
		}
		return own, nil
	})
	if err != nil {
		return nil, err
	}
	return *val, nil
}

func (l *ResourceLogic) Create(ctx context.Context, param CreateResourceParam) (*ent.Resource, error) {

	exists, err := l.Exists(ctx, param.ID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errs.ErrResourceIDAlreadyExists
	}

	return l.resourceRepo.CreateResource(ctx,
		param.ID,
		param.Name, param.Description,
		param.UpdateType,
	)
}

func (l *ResourceLogic) Exists(ctx context.Context, id string) (bool, error) {
	return l.resourceRepo.CheckResourceExistsByID(ctx, id)
}
