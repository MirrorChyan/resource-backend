package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Dropping another key while a value loads leaves that value to be kept.
func TestComputeIfAbsentKeepsValueAcrossDropOfOtherKey(t *testing.T) {
	c := NewCache[string, int](-1)

	_, err := c.ComputeIfAbsent("k", func() (int, error) {
		c.Delete("other")
		return 1, nil
	})
	require.NoError(t, err)
	got, ok := c.Get("k")
	require.True(t, ok)
	require.Equal(t, 1, got)
}

// A value loaded while its entry is dropped is served, but not kept past the drop.
func TestComputeIfAbsentKeepsNoValueLoadedAcrossDrop(t *testing.T) {
	for name, drop := range map[string]func(c *Cache[string, int]){
		"delete":    func(c *Cache[string, int]) { c.Delete("k") },
		"evict all": func(c *Cache[string, int]) { c.EvictAll() },
	} {
		t.Run(name, func(t *testing.T) {
			c := NewCache[string, int](-1)

			v, err := c.ComputeIfAbsent("k", func() (int, error) {
				drop(c)
				return 1, nil
			})
			require.NoError(t, err)
			require.Equal(t, 1, *v)
			_, ok := c.Get("k")
			require.False(t, ok)

			v, err = c.ComputeIfAbsent("k", func() (int, error) { return 2, nil })
			require.NoError(t, err)
			require.Equal(t, 2, *v)
			got, ok := c.Get("k")
			require.True(t, ok)
			require.Equal(t, 2, got)
		})
	}
}
