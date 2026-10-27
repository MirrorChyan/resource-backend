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

func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) bool {
	return c.cache.SetWithTTL(key, value, 1, ttl)
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
				c.cache.Set(key, value, 1)
			} else {
				c.cache.SetWithTTL(key, value, 1, c.ttl)
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

func NewCache[K string, V any](ttl time.Duration) *Cache[K, V] {
	cache, _ := ristretto.NewCache(&ristretto.Config[K, V]{
		NumCounters: 2048,
		MaxCost:     2048,
		BufferItems: 128,
	})
	return &Cache[K, V]{
		cache:   cache,
		group:   &singleflight.Group{},
		ttl:     ttl,
		loading: map[K]bool{},
	}
}
