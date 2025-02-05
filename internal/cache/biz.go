package cache

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"strings"
	"time"
)

type VersionCacheGroup struct {
	IncrementalUpdatePathCache *Cache[string, string]
	FullUpdatePathCache        *Cache[string, string]
	// value store pointer don't modify it
	VersionLatestCache     *Cache[string, *ent.Version]
	VersionNameCache       *Cache[string, *ent.Version]
	FullUpdateStorageCache *Cache[string, *ent.Storage]
}

func (g *VersionCacheGroup) GetCacheKey(elems ...string) string {
	return strings.Join(elems, ":")
}

func (g *VersionCacheGroup) EvictAll() {
	g.VersionLatestCache.EvictAll()
	g.VersionNameCache.EvictAll()
	g.IncrementalUpdatePathCache.EvictAll()
	g.FullUpdatePathCache.EvictAll()
	g.FullUpdateStorageCache.EvictAll()
}

func NewVersionCacheGroup(rdb *redis.Client) *VersionCacheGroup {
	group := &VersionCacheGroup{
		VersionLatestCache:         NewCache[string, *ent.Version](6 * time.Hour),
		VersionNameCache:           NewCache[string, *ent.Version](6 * time.Hour),
		IncrementalUpdatePathCache: NewCache[string, string](6 * time.Hour),
		FullUpdatePathCache:        NewCache[string, string](6 * time.Hour),
		FullUpdateStorageCache:     NewCache[string, *ent.Storage](6 * time.Hour),
	}
	subscribeCacheEvict(rdb, group)
	return group
}

func subscribeCacheEvict(rdb *redis.Client, group *VersionCacheGroup) {
	var (
		logger  = zap.L()
		cxt     = context.Background()
		channel = "evict"
	)

	subscribe := rdb.Subscribe(cxt, channel)
	go func() {
		for {
			msg, err := subscribe.ReceiveMessage(cxt)
			if err != nil {
				logger.Error("failed to receive message",
					zap.Error(err),
				)
				continue
			}
			group.EvictAll()
			logger.Info("cache evict",
				zap.String("key", msg.Payload),
			)
		}
	}()

}
