//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/google/wire"
	"go.uber.org/zap"
)

var logicSet = wire.NewSet(
	logic.NewResourceLogic,
	logic.NewVersionLogic,
	logic.NewStorageLogic,
)

var handlerSet = wire.NewSet(
	handler.NewResourceHandler,
	handler.NewVersionHandler,
)

type HandlerSet struct {
	ResourceHandler *handler.ResourceHandler
	VersionHandler  *handler.VersionHandler
}

func newHandlerSet(resourceHandler *handler.ResourceHandler, versionHandler *handler.VersionHandler) *HandlerSet {
	return &HandlerSet{
		ResourceHandler: resourceHandler,
		VersionHandler:  versionHandler,
	}
}

func NewHandlerSet(conf *config.Config, logger *zap.Logger, db *ent.Client) *HandlerSet {
	wire.Build(logicSet, handlerSet, newHandlerSet)
	return nil
}
