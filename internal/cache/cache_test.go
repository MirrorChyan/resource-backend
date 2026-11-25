package cache

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Dropping another key while a value loads leaves that value to be kept.
func TestComputeIfAbsentKeepsValueAcrossDropOfOtherKey(t *testing.T) {
	c := NewCache[string, int](-1, 16)

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
			c := NewCache[string, int](-1, 16)

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

// A cache holds as many entries as it is made for: weighing the entries of ristretto too
// left a cache of 2048 entries room for 35.
func TestCacheHoldsAsManyEntriesAsItIsMadeFor(t *testing.T) {
	c := NewCache[string, int](-1, 1000)
	for i := range 1000 {
		_, err := c.ComputeIfAbsent(strconv.Itoa(i), func() (int, error) { return i, nil })
		require.NoError(t, err)
	}
	for i := range 1000 {
		got, ok := c.Get(strconv.Itoa(i))
		require.True(t, ok, "entry %d", i)
		require.Equal(t, i, got)
	}
}

// What the entries of a weighted cache weigh stays within its maximum.
func TestWeightedCacheKeepsItsWeightWithinMax(t *testing.T) {
	c := NewWeightedCache[string, string](-1, 1000, 100, func(v string) int64 { return int64(len(v)) })
	value := strings.Repeat("x", 300)
	for i := range 10 {
		_, err := c.ComputeIfAbsent(strconv.Itoa(i), func() (string, error) { return value, nil })
		require.NoError(t, err)
	}
	kept := 0
	for i := range 10 {
		if _, ok := c.Get(strconv.Itoa(i)); ok {
			kept++
		}
	}
	require.Positive(t, kept)
	require.LessOrEqual(t, kept*len(value), 1000)
}

// An entry is gone once its time to live is up.
func TestCacheEntryExpires(t *testing.T) {
	c := NewCache[string, int](50*time.Millisecond, 16)
	_, err := c.ComputeIfAbsent("k", func() (int, error) { return 1, nil })
	require.NoError(t, err)
	_, ok := c.Get("k")
	require.True(t, ok)

	time.Sleep(100 * time.Millisecond)
	_, ok = c.Get("k")
	require.False(t, ok)
}
