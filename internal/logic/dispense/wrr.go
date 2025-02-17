package dispense

import (
	"context"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/lb"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/bytedance/sonic"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
	"strings"
)

type WeightedRoundRobinDistributor struct {
	*DistributeLogic
}

func (d *WeightedRoundRobinDistributor) Name() string {
	return "wrr"
}

func (d *WeightedRoundRobinDistributor) Distribute(info *model.DistributeInfo) (string, error) {
	d.logger.Info("Distribute Use By", zap.String("name", d.Name()))
	var (
		cfg    = config.GConfig
		ctx    = context.Background()
		region = info.Region
		time   = cfg.Extra.DownloadEffectiveTime
	)

	// The download has a more nuanced judgment so no verification <DownloadLimitCount> is required
	key := ksuid.New().String()
	sk := strings.Join([]string{misc.ResourcePrefix, key}, ":")

	value, err := sonic.Marshal(map[string]string{
		"cdk":  info.CDK,
		"path": info.RelPath,
		"rid":  info.Resource,
	})
	if err != nil {
		return "", err
	}

	_, err = d.rdb.Set(ctx, sk, value, time).Result()
	if err != nil {
		return "", err
	}

	// Acquire and Next is not atomic operation
	var (
		wrr    = lb.WRR().Acquire(region)
		prefix = wrr.Next()
	)

	root := strings.Join([]string{prefix, info.RelPath}, "/")
	url := fmt.Sprintf("%s?key=%s", root, key)

	return url, nil
}
