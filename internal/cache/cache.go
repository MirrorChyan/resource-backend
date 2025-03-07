package cache

import (
	"github.com/dgraph-io/ristretto/v2"
	"github.com/golang/groupcache/singleflight"
	"time"
)

type Cache[K string, V any] struct {
	cache *ristretto.Cache[K, V]
	group singleflight.Group
	ttl   time.Duration
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
		value, err := f()
		if err != nil {
			return nil, err
		}
		if c.ttl == -1 {
			c.cache.Set(key, value, 1)
		} else {
			c.cache.SetWithTTL(key, value, 1, c.ttl)
		}
		c.cache.Wait()
		return value, nil
	})
	if err != nil {
		return nil, err
	}
	r := cv.(V)
	return &r, nil
}

func (c *Cache[K, V]) Delete(key K) {
	c.cache.Del(key)
}

func (c *Cache[K, V]) EvictAll() {
	c.cache.Clear()
}

func NewCache[K string, V any](ttl time.Duration) *Cache[K, V] {
	cache, _ := ristretto.NewCache(&ristretto.Config[K, V]{
		NumCounters: 500,
		MaxCost:     500,
		BufferItems: 64,
	})
	return &Cache[K, V]{
		cache: cache,
		group: singleflight.Group{},
		ttl:   ttl,
	}
}
