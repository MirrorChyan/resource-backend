package lb

import (
	"sync"
)

type Server struct {
	Url    string
	Weight int
}

type WeightedRoundRobin struct {
	servers []Server
	index   int
	cw      int
	gcd     int
	mu      sync.Mutex
}

func calculate(weights []int) int {
	g := weights[0]
	for _, weight := range weights {
		for weight != 0 {
			g, weight = weight, g%weight
		}
		g = g
	}
	return g
}

func maxWeight(servers []Server) int {
	m := 0
	for _, server := range servers {
		if server.Weight > m {
			m = server.Weight
		}
	}
	return m
}

func NewWeightedRoundRobin(servers []Server) *WeightedRoundRobin {
	weights := make([]int, len(servers))
	for i, server := range servers {
		weights[i] = server.Weight
	}

	return &WeightedRoundRobin{
		servers: servers,
		gcd:     calculate(weights),
		index:   -1,
	}
}

func (wrr *WeightedRoundRobin) Next() Server {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	for {
		wrr.index = (wrr.index + 1) % len(wrr.servers)
		if wrr.index == 0 {
			wrr.cw -= wrr.gcd
			if wrr.cw <= 0 {
				wrr.cw = maxWeight(wrr.servers)
				if wrr.cw == 0 {
					return Server{}
				}
			}
		}

		if wrr.servers[wrr.index].Weight >= wrr.cw {
			return wrr.servers[wrr.index]
		}
	}
}
