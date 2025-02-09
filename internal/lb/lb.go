package lb

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"go.uber.org/zap"
	"sync"
)

const cacheSize = 1200

const key = "extra.download_prefix_info"

func init() {
	config.RegisterKeyListener(config.KeyListener{
		Key: key,
		Listener: func(any) {
			var (
				cfg  = config.GConfig
				info = cfg.Extra.DownloadPrefixInfo
			)
			Robin().UpdateServers(info[cfg.Instance.RegionId])

			zap.L().Info("LB update servers")
		},
	})
}

type WeightedRoundRobin struct {
	servers []config.RobinServer
	index   int
	cw      int
	gcd     int
	cache   chan string
	mu      sync.RWMutex
}

var Robin = sync.OnceValue(func() *WeightedRoundRobin {

	servers := config.GConfig.Extra.DownloadPrefixInfo[config.GConfig.Instance.RegionId]

	return NewWeightedRoundRobin(servers)
})

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

func NewWeightedRoundRobin(servers []config.RobinServer) *WeightedRoundRobin {
	weights := make([]int, len(servers))
	for i, server := range servers {
		weights[i] = server.Weight
	}

	ch := make(chan string, cacheSize)
	wrr := &WeightedRoundRobin{
		servers: servers,
		gcd:     calculate(weights),
		index:   -1,
		cache:   ch,
	}
	go func() {
		for {
			ch <- wrr.next()
		}
	}()

	return wrr
}

func (wrr *WeightedRoundRobin) next() string {
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

func (wrr *WeightedRoundRobin) UpdateServers(servers []config.RobinServer) {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	wrr.servers = servers
	weights := make([]int, len(servers))
	for i, server := range servers {
		weights[i] = server.Weight
	}
	wrr.gcd = calculate(weights)
	wrr.cw = maxWeight(servers)
	wrr.index = -1

	for range len(wrr.cache) + 1 {
		<-wrr.cache
	}
}

func (wrr *WeightedRoundRobin) Next() string {
	wrr.mu.RLock()
	defer wrr.mu.RUnlock()
	return <-wrr.cache
}
