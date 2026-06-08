package logic

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
)

// Read-only logic backing the admin resource-management endpoints.

func (l *ResourceLogic) ListResources(ctx context.Context, offset, limit int, idLike, nameLike string) ([]*ent.Resource, int, error) {
	total, err := l.resourceRepo.CountResources(ctx, idLike, nameLike)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*ent.Resource{}, 0, nil
	}
	items, err := l.resourceRepo.ListResources(ctx, offset, limit, idLike, nameLike)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
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

func (l *ResourceLogic) CountVersions(ctx context.Context, id string) (int, error) {
	return l.resourceRepo.CountVersions(ctx, id)
}

func (l *VersionLogic) ListByResource(ctx context.Context, resID string, offset, limit int, channel string) ([]*ent.Version, int, error) {
	total, err := l.versionRepo.CountVersionsByResource(ctx, resID, channel)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*ent.Version{}, 0, nil
	}
	items, err := l.versionRepo.ListVersionsByResource(ctx, resID, offset, limit, channel)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
