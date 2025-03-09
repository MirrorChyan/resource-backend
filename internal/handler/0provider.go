package handler

import "github.com/google/wire"

var Provider = wire.NewSet(
	NewResourceHandler,
	NewVersionHandler,
	NewStorageHandler,
	NewMetricsHandler,
	NewHeathCheckHandlerHandler,
)
