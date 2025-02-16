package dispense

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"time"
)

type ContentDeliveryNetworkDistributor struct {
	*DistributeLogic
}

func (d *ContentDeliveryNetworkDistributor) Name() string {
	return "cdn"
}

func (d *ContentDeliveryNetworkDistributor) Distribute(info *model.DistributeInfo) (string, error) {
	d.logger.Info("Distribute Use By", zap.String("name", d.Name()))

	var (
		ctx = context.Background()
		t   = time.Now().Format(time.DateOnly)
		key = strings.Join([]string{"limit", t, info.CDK}, ":")
	)

	result, err := d.rdb.Incr(ctx, key).Result()
	if err != nil {
		return "", err
	}

	if result-1 > config.GConfig.Extra.DownloadLimitCount {
		return "", misc.ResourceLimitError
	}

	url := getAuthURL(info)
	return url, nil
}

func getAuthURL(info *model.DistributeInfo) string {

	var (
		prefix = config.GConfig.Extra.CdnPrefix
		pk     = config.GConfig.Auth.PrivateKey
		now    = time.Now()
		ts     = strconv.FormatInt(now.Unix(), 10)
		rand   = ksuid.New().String()
		rel    = info.RelPath
	)
	rel = strings.Join([]string{"/", rel}, "")
	val := strings.Join([]string{rel, ts, rand, "0", pk}, "-")
	token := md5.Sum([]byte(val))
	hash := hex.EncodeToString(token[:])
	ak := strings.Join([]string{ts, rand, "0", hash}, "-")
	url := strings.Join([]string{prefix, rel}, "")
	return strings.Join([]string{url, ak}, "?auth_key=")
}
