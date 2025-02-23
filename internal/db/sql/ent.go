package sql

import (
	"entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"go.uber.org/zap"
)

func NewEntClient(drv *sql.Driver) (*ent.Client, error) {
	var opts []ent.Option
	var (
		conf = config.GConfig
	)
	if conf.Extra.SqlDebugMode {
		opts = append(opts, ent.Debug(), ent.Log(func(a ...any) {
			zap.S().Info(a...)
		}))
	}
	return ent.NewClient(append(opts, ent.Driver(drv))...), nil
}
