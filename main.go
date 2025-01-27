package main

import (
	"context"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/cache"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/db"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logger"
	"github.com/MirrorChyan/resource-backend/internal/pkg/stg"
	"github.com/MirrorChyan/resource-backend/internal/wire"
	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	_ "github.com/MirrorChyan/resource-backend/internal/banner"
	_ "net/http/pprof"
)

var (
	CTX = context.Background()
)

const BodyLimit = 50 * 1024 * 1024

func main() {

	go func() {
		runtime.SetBlockProfileRate(1)     // 开启对阻塞操作的跟踪，block
		runtime.SetMutexProfileFraction(1) // 开启对锁调用的跟踪，mutex
		log.Println(http.ListenAndServe(":6061", nil))
	}()

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get current working directory, %v", err)
	}

	conf := config.New()
	config.GlobalConfig = conf

	l := logger.New(conf)
	zap.ReplaceGlobals(l)

	mysql, err := db.NewMySQL(conf)

	if err != nil {
		l.Fatal("failed to connect to database",
			zap.Error(err),
		)
	}

	defer func(m *ent.Client) {
		if err := m.Close(); err != nil {
			l.Fatal("failed to close database")
		}
	}(mysql)

	if err := mysql.Schema.Create(CTX); err != nil {
		l.Fatal("failed creating schema resources",
			zap.Error(err),
		)
	}

	redis := db.NewRedis(conf)

	redsync := db.NewRedSync(redis)

	storage := stg.New(cwd)

	group := cache.NewVersionCacheGroup()

	handlerSet := wire.NewHandlerSet(conf, l, mysql, redis, redsync, storage, group)

	app := fiber.New(fiber.Config{
		BodyLimit:   BodyLimit,
		ProxyHeader: fiber.HeaderXForwardedFor,
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

	handlerSet.MetricsHandler.Register(r)
}
