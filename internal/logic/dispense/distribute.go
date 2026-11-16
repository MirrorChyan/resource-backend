package dispense

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
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
	return signURL(
		config.GConfig.Extra.CdnPrefix,
		config.GConfig.Auth.PrivateKey,
		info.RelPath,
		info.Filesize,
		time.Now().Unix(),
		ksuid.New().String(),
	)
}

// signURL builds a CDN URL carrying an auth_key. The CDN hashes the request
// path byte for byte, without decoding it, and clients percent-encode a
// non-ASCII path they are redirected to, so the path is escaped before it is
// both signed and sent: signing the raw path fails such a file with 403.
func signURL(prefix, pk, relPath string, filesize, ts int64, nonce string) string {
	rel := (&url.URL{Path: "/" + relPath}).EscapedPath()
	t := strconv.FormatInt(ts, 10)

	token := md5.Sum([]byte(strings.Join([]string{rel, t, nonce, "0", pk}, "-")))
	ak := strings.Join([]string{t, nonce, "0", hex.EncodeToString(token[:])}, "-")

	return prefix + rel + "?auth_key=" + ak + "&r=" + strconv.FormatInt(filesize, 10)
}
