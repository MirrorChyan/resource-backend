package logic

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"

	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/repo"

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

func (l *ResourceLogic) FindUpdateTypeById(ctx context.Context, id string) (types.Update, error) {
	key := l.cg.GetCacheKey(id)
	val, err := l.cg.ResourceInfoCache.ComputeIfAbsent(key, func() (*ent.Resource, error) {
		return l.resourceRepo.FindUpdateTypeById(ctx, id)
	})
	if err != nil {
		return "", err
	}
	return types.Update((*val).UpdateType), err
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

func (l *ResourceLogic) List(ctx context.Context, param *ListResourceParam) (*ListResourceResult, error) {

	list, total, hasMore, err := l.resourceRepo.ListResource(ctx, param.Offset, param.Limit, param.Order)
	if err != nil {
		return nil, err
	}

	return &ListResourceResult{
		List:    list,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (l *ResourceLogic) GetByID(ctx context.Context, id string) (*ent.Resource, error) {
	res, err := l.resourceRepo.GetResourceByID(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.ErrResourceNotFound
		}
		return nil, err
	}
	return res, nil
}

func (l *ResourceLogic) Update(ctx context.Context, param UpdateResourceParam) (*ent.Resource, error) {
	updated, err := l.resourceRepo.UpdateResource(
		ctx,
		param.ID,
		param.Name,
		param.Description,
		param.UpdateType,
	)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errs.ErrResourceNotFound
		}
		return nil, err
	}

	// Keep update-type lookup cache consistent with latest resource data.
	l.cg.ResourceInfoCache.Delete(l.cg.GetCacheKey(param.ID))
	return updated, nil
}

func (l *ResourceLogic) Delete(ctx context.Context, id string, force bool) error {
	hasVersions, err := l.resourceRepo.HasVersions(ctx, id)
	if err != nil {
		return err
	}
	if hasVersions && !force {
		return errs.ErrResourceDeleteConflict
	}

	if force {
		err = l.resourceRepo.ForceDeleteResourceByID(ctx, id)
	} else {
		err = l.resourceRepo.DeleteResourceByID(ctx, id)
	}
	if err != nil {
		if ent.IsNotFound(err) {
			return errs.ErrResourceNotFound
		}
		return err
	}

	l.cg.ResourceInfoCache.Delete(l.cg.GetCacheKey(id))
	return nil
}
