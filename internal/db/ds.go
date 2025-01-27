package db

import (
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	_ "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

func NewDataSource() (*ent.Client, error) {
	var (
		conf = config.CFG
	)
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=True",
		conf.Database.Username,
		conf.Database.Password,
		conf.Database.Host,
		conf.Database.Port,
		conf.Database.Name,
	)

	var opts []ent.Option

	if conf.Extra.SqlDebugMode {
		opts = append(opts, ent.Debug(), ent.Log(func(a ...any) {
			zap.S().Info(a...)
		}))
	}

	return ent.Open("mysql", dsn, opts...)
}
