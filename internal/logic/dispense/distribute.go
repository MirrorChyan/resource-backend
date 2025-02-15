package dispense

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"math/rand"
	"slices"
)

type DistributeLogic struct {
	logger       *zap.Logger
	rdb          *redis.Client
	distributors []Distributor
}

func NewDistributeLogic(
	logger *zap.Logger,
	rdb *redis.Client,
) *DistributeLogic {
	lgc := &DistributeLogic{
		logger: logger,
		rdb:    rdb,
	}
	distributors := []Distributor{
		&WeightedRoundRobinDistributor{lgc},
		&ContentDeliveryNetworkDistributor{lgc},
	}
	lgc.distributors = distributors
	return lgc
}

const totalWeight = 100

type Distributor interface {
	Distribute(info *model.DistributeInfo) (string, error)
	Name() string
}

func (l *DistributeLogic) Distribute(info *model.DistributeInfo) (string, error) {
	var (
		ds      = l.distributors
		cfg     = config.GConfig
		wrr     = ds[0]
		ratio   = cfg.Extra.DistributeCdnRatio
		regions = cfg.Extra.DistributeCdnRegion
	)

	if rand.Intn(totalWeight) < ratio && slices.Contains(regions, info.Region) {
		return ds[1].Distribute(info)
	}

	return wrr.Distribute(info)
}
