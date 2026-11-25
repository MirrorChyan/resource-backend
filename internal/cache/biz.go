package cache

import (
	"context"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/prometheus/client_golang/prometheus"
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
	group := newVersionCacheGroup()
	subscribeCacheEvict(rdb, group)
	if err := prometheus.Register(group.Collector()); err != nil {
		zap.L().Warn("failed to register cache metrics",
			zap.Error(err),
		)
	}
	return group
}

// Entries that no write of the service drops live a day: a change made on the database
// directly, or an evict message an instance missed, then shows within a day.
func newVersionCacheGroup() *MultiCacheGroup {
	return &MultiCacheGroup{
		// read by the patch builds only. A storage holds the hashes of all files of its
		// package, from KiBs to MiBs, so the cache is bounded by their estimated size.
		FullUpdateStorageCache: NewWeightedCache[string, *ent.Storage](72*time.Hour, 64<<20, 1024, storageCost),
		// the versions clients update from, as many as there are versions at most
		VersionNameIdCache: NewCache[string, int](24*time.Hour, 16384),
		// the version pairs clients ask for within the week a patch is waited for
		IncrementalUpdateInfoCache: NewCache[string, *model.IncrementalUpdateInfo](168*time.Hour, 16384),
		// one entry per platform and channel of a resource, carrying the release note
		MultiVersionInfoCache:   NewWeightedCache[string, *model.MultiVersionInfo](168*time.Hour, 32<<20, 4096, multiVersionInfoCost),
		ResourceInfoCache:       NewCache[string, *ent.Resource](24*time.Hour, 1024),
		PlatformUpdateTypeCache: NewCache[string, map[model.Platform]types.Update](24*time.Hour, 1024),
	}
}

// Collector exports the counters of the caches.
func (g *MultiCacheGroup) Collector() prometheus.Collector {
	return collector{
		"full_update_storage":     g.FullUpdateStorageCache,
		"version_name_id":         g.VersionNameIdCache,
		"incremental_update_info": g.IncrementalUpdateInfoCache,
		"multi_version_info":      g.MultiVersionInfoCache,
		"resource_info":           g.ResourceInfoCache,
		"platform_update_type":    g.PlatformUpdateTypeCache,
	}
}

// storageCost estimates the bytes a storage takes in memory, mostly its file hashes: their
// strings and up to 80 bytes of map entry and allocation each, as measured on maps decoded
// from JSON, which take 1.4 to 1.6 times their JSON.
func storageCost(s *ent.Storage) int64 {
	n := int64(512)
	if s == nil {
		return n
	}
	n += int64(len(s.Os) + len(s.Arch) + len(s.PackagePath) + len(s.PackageHashSha256) + len(s.FileType) + len(s.PackCommit))
	for k, v := range s.FileHashes {
		n += int64(len(k) + len(v) + 80)
	}
	return n
}

// multiVersionInfoCost estimates the bytes a latest version takes in memory, mostly its
// release note and custom data.
func multiVersionInfoCost(m *model.MultiVersionInfo) int64 {
	n := int64(256)
	if m == nil || m.LatestVersionInfo == nil {
		return n
	}
	i := m.LatestVersionInfo
	return n + int64(len(i.VersionName)+len(i.ReleaseNote)+len(i.CustomData)+
		len(i.OS)+len(i.Arch)+len(i.Channel)+len(i.PackageHash.String)+len(i.PackagePath.String))
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
