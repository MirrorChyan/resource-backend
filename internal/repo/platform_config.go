package repo

import (
	"context"

	"entgo.io/ent/dialect/sql/sqlgraph"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/predicate"
	"github.com/MirrorChyan/resource-backend/internal/ent/resourceplatformconfig"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
)

type PlatformConfig struct {
	*Repo
}

func NewPlatformConfig(db *Repo) *PlatformConfig {
	return &PlatformConfig{
		Repo: db,
	}
}

func platformOf(rid, os, arch string) []predicate.ResourcePlatformConfig {
	return []predicate.ResourcePlatformConfig{
		resourceplatformconfig.ResourceID(rid),
		resourceplatformconfig.Os(os),
		resourceplatformconfig.Arch(arch),
	}
}

func (r *PlatformConfig) ListByResource(ctx context.Context, rid string) ([]*ent.ResourcePlatformConfig, error) {
	return r.db.ResourcePlatformConfig.Query().
		Where(resourceplatformconfig.ResourceID(rid)).
		All(ctx)
}

func (r *PlatformConfig) Get(ctx context.Context, rid, os, arch string) (*ent.ResourcePlatformConfig, error) {
	return r.db.ResourcePlatformConfig.Query().
		Where(platformOf(rid, os, arch)...).
		Only(ctx)
}

// SetUpdateType gives the platform its own update type, creating its row when it has none.
func (r *PlatformConfig) SetUpdateType(ctx context.Context, rid, os, arch string, t types.Update) error {
	err := r.db.ResourcePlatformConfig.Create().
		SetResourceID(rid).
		SetOs(os).
		SetArch(arch).
		SetUpdateType(resourceplatformconfig.UpdateType(t)).
		Exec(ctx)
	if !sqlgraph.IsUniqueConstraintError(err) {
		return err
	}
	// the platform has its row already
	return r.db.ResourcePlatformConfig.Update().
		Where(platformOf(rid, os, arch)...).
		SetUpdateType(resourceplatformconfig.UpdateType(t)).
		Exec(ctx)
}

// Delete removes the row of the platform, if it has one.
func (r *PlatformConfig) Delete(ctx context.Context, rid, os, arch string) error {
	_, err := r.db.ResourcePlatformConfig.Delete().
		Where(platformOf(rid, os, arch)...).
		Exec(ctx)
	return err
}

// CountByResources returns the number of platform rows of each of the resources that has any.
func (r *PlatformConfig) CountByResources(ctx context.Context, rids []string) (map[string]int, error) {
	if len(rids) == 0 {
		return map[string]int{}, nil
	}
	var rows []struct {
		ResourceID string `json:"resource_id"`
		Count      int    `json:"count"`
	}
	err := r.db.ResourcePlatformConfig.Query().
		Where(resourceplatformconfig.ResourceIDIn(rids...)).
		GroupBy(resourceplatformconfig.FieldResourceID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.ResourceID] = row.Count
	}
	return counts, nil
}
