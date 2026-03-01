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
	prefix := config.GConfig.Extra.CdnPrefix
	pk := config.GConfig.Auth.PrivateKey
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	rand := ksuid.New().String()
	rel := "/" + info.RelPath

	token := md5.Sum([]byte(strings.Join([]string{rel, ts, rand, "0", pk}, "-")))
	ak := strings.Join([]string{ts, rand, "0", hex.EncodeToString(token[:])}, "-")

	return prefix + rel + "?auth_key=" + ak + "&r=" + strconv.FormatInt(info.Filesize, 10)
}
