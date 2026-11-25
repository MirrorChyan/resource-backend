package cache

import (
	"github.com/dgraph-io/ristretto/v2"
	"github.com/prometheus/client_golang/prometheus"
)

// statsSource is a cache whose counters are exported.
type statsSource interface {
	stats() *ristretto.Metrics
	maxCost() int64
}

var (
	cacheLabels = []string{"cache"}

	descHits = prometheus.NewDesc("resource_cache_hits",
		"Lookups that found their key, since the start or the last evict of all entries.", cacheLabels, nil)
	descMisses = prometheus.NewDesc("resource_cache_misses",
		"Lookups that did not find their key, since the start or the last evict of all entries.", cacheLabels, nil)
	descEntries = prometheus.NewDesc("resource_cache_entries",
		"Entries the cache holds.", cacheLabels, nil)
	descCost = prometheus.NewDesc("resource_cache_cost",
		"What the entries weigh in all: their number, or their estimated bytes in a weighted cache.", cacheLabels, nil)
	descMaxCost = prometheus.NewDesc("resource_cache_max_cost",
		"What the entries may weigh in all.", cacheLabels, nil)
	descRemoved = prometheus.NewDesc("resource_cache_removed",
		"Entries evicted, expired or dropped, since the start or the last evict of all entries.", cacheLabels, nil)
	descRejected = prometheus.NewDesc("resource_cache_sets_rejected",
		"Values not kept as their keys were used less than the ones they would have evicted.", cacheLabels, nil)
	descDropped = prometheus.NewDesc("resource_cache_sets_dropped",
		"Values dropped under contention before the cache weighed them.", cacheLabels, nil)

	cacheDescs = []*prometheus.Desc{
		descHits, descMisses, descEntries, descCost, descMaxCost, descRemoved, descRejected, descDropped,
	}
)

// collector exports the counters of caches by name. They are gauges: evicting all entries
// of a cache starts its counters again from zero.
type collector map[string]statsSource

func (c collector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range cacheDescs {
		ch <- d
	}
}

func (c collector) Collect(ch chan<- prometheus.Metric) {
	for name, s := range c {
		m := s.stats()
		gauge := func(d *prometheus.Desc, v float64) {
			ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, name)
		}
		gauge(descHits, float64(m.Hits()))
		gauge(descMisses, float64(m.Misses()))
		gauge(descEntries, float64(difference(m.KeysAdded(), m.KeysEvicted())))
		gauge(descCost, float64(difference(m.CostAdded(), m.CostEvicted())))
		gauge(descMaxCost, float64(s.maxCost()))
		gauge(descRemoved, float64(m.KeysEvicted()))
		gauge(descRejected, float64(m.SetsRejected()))
		gauge(descDropped, float64(m.SetsDropped()))
	}
}

// difference is a - b, or 0 when the counters, read one after the other, have b ahead.
func difference(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}
