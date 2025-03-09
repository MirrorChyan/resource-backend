package cache

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type MultiCacheGroup struct {
	// value store pointer don't modify it

	// key: resourceId:versionName -> versionId
	VersionNameIdCache *Cache[string, int]
	// key: versionId:os:arch
	FullUpdateStorageCache *Cache[string, *ent.Storage]

	// key: targetVersionId:currentVersionId:os:arch / cache empty
	IncrementalUpdateInfoCache *Cache[string, *model.IncrementalUpdateInfo]

	// resourceId:os:arch:channel / cache empty
	MultiVersionInfoCache *Cache[string, *model.MultiVersionInfo]

	ResourceInfoCache *Cache[string, *ent.Resource]
}

func (g *MultiCacheGroup) GetCacheKey(elems ...string) string {
	return strings.Join(elems, ":")
}

func (g *MultiCacheGroup) EvictAll() {
	g.FullUpdateStorageCache.EvictAll()
	g.VersionNameIdCache.EvictAll()
	g.IncrementalUpdateInfoCache.EvictAll()
	g.MultiVersionInfoCache.EvictAll()
	g.ResourceInfoCache.EvictAll()
}

func NewVersionCacheGroup(rdb *redis.Client) *MultiCacheGroup {
	group := &MultiCacheGroup{
		FullUpdateStorageCache:     NewCache[string, *ent.Storage](12 * time.Hour),
		VersionNameIdCache:         NewCache[string, int](-1),
		IncrementalUpdateInfoCache: NewCache[string, *model.IncrementalUpdateInfo](12 * time.Hour),
		MultiVersionInfoCache:      NewCache[string, *model.MultiVersionInfo](12 * time.Hour),
		ResourceInfoCache:          NewCache[string, *ent.Resource](-1),
	}
	subscribeCacheEvict(rdb, group)
	return group
}

func subscribeCacheEvict(rdb *redis.Client, group *MultiCacheGroup) {
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
