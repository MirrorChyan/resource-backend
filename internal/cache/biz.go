package cache

import (
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"time"
)

type VersionCacheGroup struct {
	VersionLatestCache *Cache[string, *ent.Version]
	VersionNameCache   *Cache[string, *ent.Version]
}

func NewVersionCacheGroup() *VersionCacheGroup {
	return &VersionCacheGroup{
		VersionLatestCache: NewCache[string, *ent.Version](6 * time.Hour),
		VersionNameCache:   NewCache[string, *ent.Version](6 * time.Hour),
	}
}
