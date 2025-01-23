//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/pkg/stg"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var repoProviderSet = wire.NewSet(
	repo.NewResource,
	repo.NewVersion,
	repo.NewStorage,
	repo.NewTempDownloadInfo,
)

var logicProviderSet = wire.NewSet(
	logic.NewResourceLogic,
	logic.NewVersionLogic,
)

var handlerProviderSet = wire.NewSet(
	handler.NewResourceHandler,
	handler.NewVersionHandler,
	handler.NewMetricsHandler,
)

type HandlerSet struct {
	ResourceHandler *handler.ResourceHandler
	VersionHandler  *handler.VersionHandler
	MetricsHandler  *handler.MetricsHandler
}

func provideHandlerSet(resourceHandler *handler.ResourceHandler, versionHandler *handler.VersionHandler) *HandlerSet {
	return &HandlerSet{
		ResourceHandler: resourceHandler,
		VersionHandler:  versionHandler,
	}
}

func NewHandlerSet(conf *config.Config, logger *zap.Logger, db *ent.Client, rdb *redis.Client, storage *stg.Storage) *HandlerSet {
	panic(wire.Build(repoProviderSet, logicProviderSet, handlerProviderSet, provideHandlerSet))
}
