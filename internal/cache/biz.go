package cache

import (
	"context"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"

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

	// key: resourceId -> update types of the platforms that have their own / cache empty;
	// readers share the map, never modify it
	PlatformUpdateTypeCache *Cache[string, map[model.Platform]types.Update]
}

// evictChannel tells every instance to drop all of its caches
const evictChannel = "evict"

// platformUpdateTypeEvictChannel tells every instance to drop the platform update types
// of the resource the message names
const platformUpdateTypeEvictChannel = "evict:platform-update-type"

func (g *MultiCacheGroup) GetCacheKey(elems ...string) string {
	return strings.Join(elems, ":")
}

func (g *MultiCacheGroup) EvictAll() {
	g.FullUpdateStorageCache.EvictAll()
	g.VersionNameIdCache.EvictAll()
	g.IncrementalUpdateInfoCache.EvictAll()
	g.MultiVersionInfoCache.EvictAll()
	g.ResourceInfoCache.EvictAll()
	g.PlatformUpdateTypeCache.EvictAll()
}

// PublishPlatformUpdateTypeEvict asks every instance, this one included, to drop the
// cached platform update types of the resource.
func PublishPlatformUpdateTypeEvict(ctx context.Context, rdb *redis.Client, resourceId string) error {
	return rdb.Publish(ctx, platformUpdateTypeEvictChannel, resourceId).Err()
}

func NewVersionCacheGroup(rdb *redis.Client) *MultiCacheGroup {
	group := &MultiCacheGroup{
		FullUpdateStorageCache:     NewCache[string, *ent.Storage](72 * time.Hour),
		VersionNameIdCache:         NewCache[string, int](-1),
		IncrementalUpdateInfoCache: NewCache[string, *model.IncrementalUpdateInfo](168 * time.Hour),
		MultiVersionInfoCache:      NewCache[string, *model.MultiVersionInfo](168 * time.Hour),
		ResourceInfoCache:          NewCache[string, *ent.Resource](-1),
		PlatformUpdateTypeCache:    NewCache[string, map[model.Platform]types.Update](-1),
	}
	subscribeCacheEvict(rdb, group)
	return group
}

func subscribeCacheEvict(rdb *redis.Client, group *MultiCacheGroup) {
	var (
		logger = zap.L()
		cxt    = context.Background()
	)

	subscribe := rdb.Subscribe(cxt, evictChannel, platformUpdateTypeEvictChannel)
	go func() {
		for {
			msg, err := subscribe.ReceiveMessage(cxt)
			if err != nil {
				logger.Error("failed to receive message",
					zap.Error(err),
				)
				continue
			}
			if msg.Channel == platformUpdateTypeEvictChannel {
				group.PlatformUpdateTypeCache.Delete(group.GetCacheKey(msg.Payload))
				logger.Info("platform update type cache evict",
					zap.String("resource id", msg.Payload),
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
