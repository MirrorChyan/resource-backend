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
	"go.uber.org/zap"
)

var repoProviderSet = wire.NewSet(
	repo.NewResource,
	repo.NewVersion,
	repo.NewStorage,
)

var logicProviderSet = wire.NewSet(
	logic.NewResourceLogic,
	logic.NewVersionLogic,
)

var handlerProviderSet = wire.NewSet(
	handler.NewResourceHandler,
	handler.NewVersionHandler,
)

type HandlerSet struct {
	ResourceHandler *handler.ResourceHandler
	VersionHandler  *handler.VersionHandler
}

func provideHandlerSet(resourceHandler *handler.ResourceHandler, versionHandler *handler.VersionHandler) *HandlerSet {
	return &HandlerSet{
		ResourceHandler: resourceHandler,
		VersionHandler:  versionHandler,
	}
}

func NewHandlerSet(conf *config.Config, logger *zap.Logger, db *ent.Client, storage *stg.Storage) *HandlerSet {
	panic(wire.Build(repoProviderSet, logicProviderSet, handlerProviderSet, provideHandlerSet))
}
