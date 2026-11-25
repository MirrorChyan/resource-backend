package cache

import (
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/golang/groupcache/singleflight"
)

type Cache[K string, V any] struct {
	cache *ristretto.Cache[K, V]
	group *singleflight.Group
	ttl   time.Duration

	// loading holds the keys being loaded, set to true once the key is dropped meanwhile:
	// such a value may predate the change the drop was made for, so it is served but not kept
	mu      sync.Mutex
	loading map[K]bool
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	return c.cache.Get(key)
}

// SetWithTTL stores value for ttl; it weighs what the cache was made to weigh its values.
func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) bool {
	return c.cache.SetWithTTL(key, value, 0, ttl)
}

func (c *Cache[K, V]) ComputeIfAbsent(key K, f func() (V, error)) (*V, error) {
	v, ok := c.cache.Get(key)
	if ok {
		return &v, nil
	}
	cv, err := c.group.Do(string(key), func() (any, error) {
		c.mu.Lock()
		c.loading[key] = false
		c.mu.Unlock()

		value, err := f()

		c.mu.Lock()
		store := err == nil && !c.loading[key]
		delete(c.loading, key)
		if store {
			if c.ttl == -1 {
				c.cache.Set(key, value, 0)
			} else {
				c.cache.SetWithTTL(key, value, 0, c.ttl)
			}
		}
		c.mu.Unlock()

		if err != nil {
			return nil, err
		}
		if store {
			// a later Del or Clear is queued behind the set, so waiting outside the lock keeps the order
			c.cache.Wait()
		}
		return value, nil
	})
	if err != nil {
		return nil, err
	}
	r := cv.(V)
	return &r, nil
}

func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.loading[key]; ok {
		c.loading[key] = true
	}
	c.cache.Del(key)
}

func (c *Cache[K, V]) EvictAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.loading {
		c.loading[key] = true
	}
	c.cache.Clear()
}

// stats are the counters of the cache; EvictAll starts them again from zero.
func (c *Cache[K, V]) stats() *ristretto.Metrics {
	return c.cache.Metrics
}

func (c *Cache[K, V]) maxCost() int64 {
	return c.cache.MaxCost()
}

// NewCache returns a cache of maxEntries entries at most. Entries live for ttl, or until
// they are dropped or evicted when ttl is -1.
func NewCache[K string, V any](ttl time.Duration, maxEntries int64) *Cache[K, V] {
	return newCache[K, V](ttl, maxEntries, maxEntries, func(V) int64 { return 1 })
}

// NewWeightedCache returns a cache whose entries weigh what cost estimates of their values,
// maxCost at most in all. keys is the number of entries it holds when full, which sizes the
// counters it decides what to keep by.
func NewWeightedCache[K string, V any](ttl time.Duration, maxCost, keys int64, cost func(V) int64) *Cache[K, V] {
	return newCache[K, V](ttl, maxCost, keys, cost)
}

func newCache[K string, V any](ttl time.Duration, maxCost, keys int64, cost func(V) int64) *Cache[K, V] {
	cache, err := ristretto.NewCache(&ristretto.Config[K, V]{
		// ristretto advises ten counters per entry of a full cache
		NumCounters: keys * 10,
		MaxCost:     maxCost,
		BufferItems: 64,
		// values are set with a cost of 0 for ristretto to weigh them with cost; it must not
		// add the size of its own entry to that weight, which made a cache of 2048 entries
		// hold 35
		Cost:               cost,
		IgnoreInternalCost: true,
		Metrics:            true,
	})
	if err != nil {
		panic(err)
	}
	return &Cache[K, V]{
		cache:   cache,
		group:   &singleflight.Group{},
		ttl:     ttl,
		loading: map[K]bool{},
	}
}