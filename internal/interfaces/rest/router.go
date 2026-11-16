package rest

import (
	"github.com/MirrorChyan/resource-backend/internal/interfaces/rest/handler"
	"github.com/MirrorChyan/resource-backend/internal/wire"
	"github.com/bytedance/sonic"
	fiberzap "github.com/gofiber/contrib/v3/zap"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

const BodyLimit = 1000 * 1024 * 1024

func NewRouter() *fiber.App {

	router := fiber.New(fiber.Config{
		AppName:   "resource-backend",
		BodyLimit: BodyLimit,

		// The gateway reaches the service over a private network. Without
		// EnableIPValidation, c.IP() returns the raw X-Forwarded-For value as v2 did.
		TrustProxy:  true,
		ProxyHeader: fiber.HeaderXForwardedFor,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Loopback: true,
			Private:  true,
		},

		// A generated HEAD route runs the whole GET chain, which on
		// /resources/:rid/latest records DAU and dispenses a download key.
		DisableHeadAutoRegister: true,

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

	handlerSet.AdminHandler.Register(r)

	handlerSet.StorageHandler.Register(r)

	handlerSet.MetricsHandler.Register(r)

	handlerSet.HeathCheckHandler.Register(r)
}
