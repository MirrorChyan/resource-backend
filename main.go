package main

import (
	"context"
	"fmt"

	_ "github.com/MirrorChyan/resource-backend/internal/banner"
	"github.com/MirrorChyan/resource-backend/internal/cache"
	. "github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/db"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logger"
	"github.com/MirrorChyan/resource-backend/internal/vercomp"
	"github.com/MirrorChyan/resource-backend/internal/wire"
	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

const BodyLimit = 1000 * 1024 * 1024

func main() {

	setUpConfigAndLog()

	mysql, err := db.NewDataSource()

	if err != nil {
		zap.L().Fatal("failed to connect to database",
			zap.Error(err),
		)
	}

	defer func(m *ent.Client) {
		if err := m.Close(); err != nil {
			zap.L().Fatal("failed to close database")
		}
	}(mysql)

	if err := mysql.Schema.Create(context.Background()); err != nil {
		zap.L().Fatal("failed creating schema resources",
			zap.Error(err),
		)
	}

	// deps
	var (
		redis         = db.NewRedis()
		redSync       = db.NewRedSync(redis)
		group         = cache.NewVersionCacheGroup(redis)
		verComparator = vercomp.NewComparator()
		app           = fiber.New(fiber.Config{
			BodyLimit:   BodyLimit,
			ProxyHeader: fiber.HeaderXForwardedFor,
			JSONEncoder: sonic.Marshal,
			JSONDecoder: sonic.Unmarshal,
		})
	)

	handlerSet := wire.NewHandlerSet(zap.L(), mysql, redis, redSync, group, verComparator)

	initRoute(app, handlerSet)

	addr := fmt.Sprintf(":%d", GConfig.Instance.Port)

	if err := app.Listen(addr); err != nil {
		zap.L().Fatal("failed to start server",
			zap.Error(err),
		)
	}

}

func setUpConfigAndLog() {
	// in the full life cycle
	InitGlobalConfig()
	zap.ReplaceGlobals(logger.New())
}

func initRoute(app *fiber.App, handlerSet *wire.HandlerSet) {
	app.Use(fiberzap.New(fiberzap.Config{
		Logger: zap.L(),
		SkipURIs: []string{
			"/metrics",
			"/health",
		},
	}))

	r := app.Group("/")

	handlerSet.ResourceHandler.Register(r)

	handlerSet.VersionHandler.Register(r)

	handlerSet.StorageHandler.Register(r)

	handlerSet.MetricsHandler.Register(r)

	handlerSet.HeathCheckHandler.Register(r)
}
