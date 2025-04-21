package rest

import (
	"github.com/MirrorChyan/resource-backend/internal/interfaces/rest/handler"
	"github.com/MirrorChyan/resource-backend/internal/wire"
	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/fiberzap/v2"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

const BodyLimit = 1000 * 1024 * 1024

func NewRouter() *fiber.App {

	router := fiber.New(fiber.Config{
		AppName:     "resource-backend",
		BodyLimit:   BodyLimit,
		ProxyHeader: fiber.HeaderXForwardedFor,

		JSONEncoder: sonic.Marshal,
		JSONDecoder: sonic.Unmarshal,

		ErrorHandler: handler.Error,
	})

	return router
}

func InitRoutes(router *fiber.App, handlerSet *wire.HandlerSet) {

	router.Use(fiberzap.New(fiberzap.Config{
		Logger: zap.L(),
		SkipURIs: []string{
			"/metrics",
			"/health",
		},
	}))

	r := router.Group("/")

	handlerSet.ResourceHandler.Register(r)

	handlerSet.VersionHandler.Register(r)

	handlerSet.StorageHandler.Register(r)

	handlerSet.MetricsHandler.Register(r)

	handlerSet.HeathCheckHandler.Register(r)
}
