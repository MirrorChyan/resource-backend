package restserver

import (
	"context"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/application"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/gofiber/fiber/v2"
)

func NewAdapter(restServer *fiber.App) application.Adapter {
	return &Adapter{
		restServer: restServer,
	}
}

type Adapter struct {
	restServer *fiber.App
}

func (a Adapter) Start(ctx context.Context) error {

	addr := fmt.Sprintf(":%d", config.GConfig.Instance.Port)
	return a.restServer.Listen(addr)
}

func (a Adapter) Stop(ctx context.Context) error {

	return a.restServer.Shutdown()
}
