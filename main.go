package main

import (
	"context"
	"fmt"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/db"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logger"
	"github.com/MirrorChyan/resource-backend/internal/wire"
	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	_ "github.com/MirrorChyan/resource-backend/internal/banner"
)

var (
	CTX = context.Background()
)

const BodyLimit = 50 * 1024 * 1024

func main() {
	conf := config.New()
	config.GlobalConfig = conf

	l := logger.New(conf)
	zap.ReplaceGlobals(l)

	db.NewRedis(conf)

	mySQL, err := db.NewMySQL(conf)

	if err != nil {
		l.Fatal("failed to connect to database",
			zap.Error(err),
		)
	}

	defer func(mySQL *ent.Client) {
		err := mySQL.Close()
		if err != nil {
			l.Fatal("failed to close database")
		}
	}(mySQL)

	if err := mySQL.Schema.Create(CTX); err != nil {
		l.Fatal("failed creating schema resources",
			zap.Error(err),
		)
	}

	handlerSet := wire.NewHandlerSet(conf, l, mySQL)

	app := fiber.New(fiber.Config{
		BodyLimit: BodyLimit,
	})
	app.Use(fiberzap.New(fiberzap.Config{
		Logger: l,
	}))

	initRoute(app, handlerSet)

	addr := fmt.Sprintf(":%d", conf.Server.Port)

	if err := app.Listen(addr); err != nil {
		l.Fatal("failed to start server",
			zap.Error(err),
		)
	}

}

func initRoute(app *fiber.App, handlerSet *wire.HandlerSet) {
	r := app.Group("/")

	handlerSet.ResourceHandler.Register(r)

	handlerSet.VersionHandler.Register(r)
}
