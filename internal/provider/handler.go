package provider

import (
	"github.com/MirrorChyan/resource-backend/internal/handler"
	"github.com/google/wire"
)

var HandlerSet = wire.NewSet(
	handler.NewResourceHandler,
	handler.NewVersionHandler,
	handler.NewMetricsHandler,
	handler.NewHeathCheckHandlerHandler,
)
