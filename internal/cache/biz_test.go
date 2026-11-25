package cache

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func requireHolds[V any](t *testing.T, name string, c *Cache[string, V], value V, n int) {
	t.Helper()
	for i := range n {
		_, err := c.ComputeIfAbsent(strconv.Itoa(i), func() (V, error) { return value, nil })
		require.NoError(t, err)
	}
	for i := range n {
		_, ok := c.Get(strconv.Itoa(i))
		require.True(t, ok, "%s lost entry %d", name, i)
	}
}

// Every cache of the group holds a thousand small entries; each was sized for more.
func TestVersionCacheGroupHoldsAThousandEntriesEach(t *testing.T) {
	g := newVersionCacheGroup()
	requireHolds(t, "full update storage", g.FullUpdateStorageCache, &ent.Storage{}, 1000)
	requireHolds(t, "version name id", g.VersionNameIdCache, 1, 1000)
	requireHolds(t, "incremental update info", g.IncrementalUpdateInfoCache, &model.IncrementalUpdateInfo{}, 1000)
	requireHolds(t, "multi version info", g.MultiVersionInfoCache, &model.MultiVersionInfo{}, 1000)
	requireHolds(t, "resource info", g.ResourceInfoCache, &ent.Resource{}, 1000)
	requireHolds(t, "platform update type", g.PlatformUpdateTypeCache, map[model.Platform]types.Update{}, 1000)
}

// A storage weighs more than the strings of its file hashes, so that its estimate does not
// fall short of what it takes in memory.
func TestStorageCostCountsFileHashes(t *testing.T) {
	hashes := make(map[string]string, 1000)
	var size int64
	for i := range 1000 {
		k, v := fmt.Sprintf("bin/file-%d.dll", i), strings.Repeat("a", 64)
		hashes[k] = v
		size += int64(len(k) + len(v))
	}
	require.Greater(t, storageCost(&ent.Storage{FileHashes: hashes}), size)
	require.Less(t, storageCost(&ent.Storage{}), int64(1024))
}

func TestMultiVersionInfoCostCountsReleaseNote(t *testing.T) {
	note := strings.Repeat("n", 10000)
	got := multiVersionInfoCost(&model.MultiVersionInfo{LatestVersionInfo: &model.LatestVersionInfo{ReleaseNote: note}})
	require.Greater(t, got, int64(len(note)))
	require.Less(t, multiVersionInfoCost(&model.MultiVersionInfo{}), int64(1024))
}

// The metrics name every cache of the group and follow its entries and lookups.
func TestCollectorExportsEveryCache(t *testing.T) {
	g := newVersionCacheGroup()
	for range 2 {
		_, err := g.MultiVersionInfoCache.ComputeIfAbsent("k", func() (*model.MultiVersionInfo, error) {
			return &model.MultiVersionInfo{}, nil
		})
		require.NoError(t, err)
	}

	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(g.Collector()))
	families, err := reg.Gather()
	require.NoError(t, err)

	values := map[string]map[string]float64{}
	for _, f := range families {
		byCache := map[string]float64{}
		for _, m := range f.GetMetric() {
			byCache[m.GetLabel()[0].GetValue()] = m.GetGauge().GetValue()
		}
		values[f.GetName()] = byCache
	}
	require.Len(t, values, len(cacheDescs))
	require.Len(t, values["resource_cache_entries"], 6)
	require.Equal(t, 1.0, values["resource_cache_entries"]["multi_version_info"])
	require.Equal(t, 1.0, values["resource_cache_hits"]["multi_version_info"])
	require.Equal(t, 1.0, values["resource_cache_misses"]["multi_version_info"])
	require.Equal(t, float64(32<<20), values["resource_cache_max_cost"]["multi_version_info"])
	require.Equal(t, 1024.0, values["resource_cache_max_cost"]["resource_info"])
}
