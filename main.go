package main

import (
	"context"
	"fmt"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/db"
	"github.com/MirrorChyan/resource-backend/internal/logger"
	"github.com/MirrorChyan/resource-backend/internal/wire"
	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

var (
	CTX = context.Background()
)

const BodyLimit = 50 * 1024 * 1024

func main() {
	conf := config.New()
	l := logger.New(conf)

	database, err := db.New(conf)
	if err != nil {
		l.Fatal("failed to connect to database",
			zap.Error(err),
		)
	}
	defer database.Close()
	if err := database.Schema.Create(CTX); err != nil {
		l.Fatal("failed creating schema resources",
			zap.Error(err))
	}

	handlerSet := wire.NewHandlerSet(l, database)

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
