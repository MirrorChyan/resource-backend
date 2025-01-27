package db

import (
	"fmt"
	"github.com/gofiber/fiber/v2/log"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"

	_ "github.com/go-sql-driver/mysql"
)

func NewMySQL(conf *config.Config) (*ent.Client, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=True",
		conf.Database.Username,
		conf.Database.Password,
		conf.Database.Host,
		conf.Database.Port,
		conf.Database.Name,
	)

	return ent.Open("mysql", dsn, ent.Debug(), ent.Log(func(a ...any) {
		log.Info(a...)
	}))
}
