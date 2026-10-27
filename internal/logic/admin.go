package logic

import (
	"cmp"
	"context"
	"slices"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"go.uber.org/zap"
)

// Logic backing the admin resource-management endpoints.

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

// CountPlatformOverrides returns the number of platforms that have their own update type
// of each of the resources that has any.
func (l *ResourceLogic) CountPlatformOverrides(ctx context.Context, ids []string) (map[string]int, error) {
	return l.platformRepo.CountByResources(ctx, ids)
}

// ListPlatformUpdateTypes lists the update type in effect for the platforms of the resource:
// the ones packages have been uploaded for and the ones that have their own. It reads the
// database rather than the cache, so a change shows at once.
func (l *ResourceLogic) ListPlatformUpdateTypes(ctx context.Context, id string) (*PlatformUpdateTypesData, error) {
	res, err := l.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := l.platformRepo.ListByResource(ctx, res.ID)
	if err != nil {
		return nil, err
	}
	stored, err := l.storageRepo.ListPlatforms(ctx, res.ID)
	if err != nil {
		return nil, err
	}

	own := make(map[Platform]*ent.ResourcePlatformConfig, len(rows))
	for _, row := range rows {
		own[Platform{OS: row.Os, Arch: row.Arch}] = row
	}
	hasStorage := make(map[Platform]bool, len(stored))
	for _, p := range stored {
		hasStorage[p] = true
	}

	list := make([]PlatformUpdateTypeItem, 0, len(rows)+len(stored))
	add := func(p Platform) {
		row := own[p]
		item := PlatformUpdateTypeItem{
			OS:                      p.OS,
			Arch:                    p.Arch,
			PlatformUpdateTypeState: platformState(res, ownUpdateType(row)),
			HasStorage:              hasStorage[p],
		}
		if row != nil {
			item.UpdatedAt = &row.UpdatedAt
		}
		list = append(list, item)
	}
	for p := range own {
		add(p)
	}
	for p := range hasStorage {
		if own[p] == nil {
			add(p)
		}
	}
	slices.SortFunc(list, func(a, b PlatformUpdateTypeItem) int {
		return cmp.Or(cmp.Compare(a.OS, b.OS), cmp.Compare(a.Arch, b.Arch))
	})
	return &PlatformUpdateTypesData{
		DefaultUpdateType: res.UpdateType,
		Platforms:         list,
	}, nil
}

// SetPlatformUpdateType gives the platform of the resource its own update type.
func (l *ResourceLogic) SetPlatformUpdateType(ctx context.Context, id, os, arch string, t types.Update) (*PlatformUpdateTypeChangeData, error) {
	return l.changePlatformUpdateType(ctx, id, os, arch, t)
}

// DeletePlatformUpdateType returns the platform of the resource to the resource's update type;
// a platform without its own is left as is.
func (l *ResourceLogic) DeletePlatformUpdateType(ctx context.Context, id, os, arch string) (*PlatformUpdateTypeChangeData, error) {
	return l.changePlatformUpdateType(ctx, id, os, arch, "")
}

// changePlatformUpdateType gives the platform its own update type own, or returns it to
// the resource's when own is "".
func (l *ResourceLogic) changePlatformUpdateType(ctx context.Context, id, os, arch string, own types.Update) (*PlatformUpdateTypeChangeData, error) {
	res, err := l.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	row, err := l.platformRepo.Get(ctx, res.ID, os, arch)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if own == "" {
		err = l.platformRepo.Delete(ctx, res.ID, os, arch)
	} else {
		err = l.platformRepo.SetUpdateType(ctx, res.ID, os, arch, own)
	}
	if err != nil {
		return nil, err
	}
	l.evictPlatformUpdateTypes(ctx, res.ID)

	return &PlatformUpdateTypeChangeData{
		OS:     os,
		Arch:   arch,
		Before: platformState(res, ownUpdateType(row)),
		After:  platformState(res, own),
	}, nil
}

// ownUpdateType is the update type the platform row gives its platform, "" without a row.
func ownUpdateType(row *ent.ResourcePlatformConfig) types.Update {
	if row == nil {
		return ""
	}
	return types.Update(row.UpdateType)
}

// platformState is the update type in effect for a platform: its own when it has one,
// the resource's otherwise. ResolveUpdateType applies the same rule to serve updates.
func platformState(res *ent.Resource, own types.Update) PlatformUpdateTypeState {
	if own != "" {
		return PlatformUpdateTypeState{UpdateType: own.String(), Source: PlatformSourceOverride}
	}
	return PlatformUpdateTypeState{UpdateType: res.UpdateType, Source: PlatformSourceInherit}
}

// evictPlatformUpdateTypes drops the cached platform update types of the resource here
// at once and on the other instances through a message to them.
func (l *ResourceLogic) evictPlatformUpdateTypes(ctx context.Context, id string) {
	l.cg.PlatformUpdateTypeCache.Delete(l.cg.GetCacheKey(id))
	if err := cache.PublishPlatformUpdateTypeEvict(ctx, l.rdb, id); err != nil {
		// the entry never expires: the other instances keep the old update types
		// until the next evict or restart
		l.logger.Warn("failed to publish platform update type cache evict",
			zap.String("resource id", id),
			zap.Error(err),
		)
	}
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
