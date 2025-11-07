package dispense

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
)

type DistributeLogic struct {
	logger *zap.Logger
	rdb    *redis.Client
}

func NewDistributeLogic(
	logger *zap.Logger,
	rdb *redis.Client,
) *DistributeLogic {
	return &DistributeLogic{
		logger: logger,
		rdb:    rdb,
	}
}

type Distributor interface {
	Distribute(info *model.DistributeInfo) (string, error)
	Name() string
}

func (d *DistributeLogic) Name() string {
	return "cdn"
}

func (d *DistributeLogic) Distribute(info *model.DistributeInfo) (string, error) {
	d.logger.Info("Distribute Use By", zap.String("name", d.Name()))
	return getAuthURL(info), nil
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
