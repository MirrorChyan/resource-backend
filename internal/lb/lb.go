package lb

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"go.uber.org/zap"
	"sync"
)

const cacheSize = 100

const key = "extra.download_prefix_info"

func init() {
	config.RegisterKeyListener(config.KeyListener{
		Key: key,
		Listener: func(any) {
			var (
				cfg  = config.GConfig
				info = cfg.Extra.DownloadPrefixInfo
			)

			WRR().Update(info)

			zap.L().Info("LB update servers")
		},
	})
}

type store struct {
	m  map[string]*robin
	mu sync.RWMutex
}

func (s *store) Acquire(region string) *robin {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.m[region]
	if !ok {
		r, _ = s.m[config.DefaultRegion]
		return r
	}
	return r
}

// Update There may be concurrency issues use it during off-peak hours
func (s *store) Update(info map[string][]config.RobinServer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.m {
		v.close()
	}
	clear(s.m)
	for region, servers := range info {
		s.m[region] = newWeightedRoundRobin(servers)
	}

}

type Robin interface {
	Acquire(string) *robin
	Update(map[string][]config.RobinServer)
}

var WRR = sync.OnceValue(func() Robin {
	regions := config.GConfig.Extra.DownloadPrefixInfo
	s := &store{
		m:  make(map[string]*robin),
		mu: sync.RWMutex{},
	}
	s.Update(regions)
	return s
})

type robin struct {
	servers []config.RobinServer
	index   int
	cw      int
	gcd     int
	cache   chan string
	done    chan struct{}
	mu      sync.RWMutex
}

func calculate(weights []int) int {
	g := weights[0]
	for _, weight := range weights {
		for weight != 0 {
			g, weight = weight, g%weight
		}
	}
	return g
}

func maxWeight(servers []config.RobinServer) int {
	m := 0
	for _, server := range servers {
		if server.Weight > m {
			m = server.Weight
		}
	}
	return m
}

func newWeightedRoundRobin(servers []config.RobinServer) *robin {
	weights := make([]int, len(servers))
	for i, server := range servers {
		weights[i] = server.Weight
	}

	ch := make(chan string, cacheSize)
	wrr := &robin{
		servers: servers,
		gcd:     calculate(weights),
		index:   -1,
		cache:   ch,
		done:    make(chan struct{}),
	}
	go func() {
		for {
			select {
			case <-wrr.done:
				return
			case ch <- wrr.next():
			}
		}
	}()

	return wrr
}

func (wrr *robin) next() string {
	for {
		wrr.index = (wrr.index + 1) % len(wrr.servers)
		if wrr.index == 0 {
			wrr.cw -= wrr.gcd
			if wrr.cw <= 0 {
				wrr.cw = maxWeight(wrr.servers)
				if wrr.cw == 0 {
					return wrr.servers[0].Url
				}
			}
		}

		if wrr.servers[wrr.index].Weight >= wrr.cw {
			return wrr.servers[wrr.index].Url
		}
	}
}

func (wrr *robin) close() {
	wrr.done <- struct{}{}
	close(wrr.done)
	close(wrr.cache)
}

func (wrr *robin) Next() string {
	return <-wrr.cache
}
