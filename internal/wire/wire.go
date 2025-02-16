//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler"
	"github.com/MirrorChyan/resource-backend/internal/provider"
	"github.com/MirrorChyan/resource-backend/internal/vercomp"
	"github.com/go-redsync/redsync/v4"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type HandlerSet struct {
	ResourceHandler   *handler.ResourceHandler
	VersionHandler    *handler.VersionHandler
	StorageHandler    *handler.StorageHandler
	MetricsHandler    *handler.MetricsHandler
	HeathCheckHandler *handler.HeathCheckHandler
}

func NewHandlerSet(
	logger *zap.Logger,
	db *ent.Client,
	rdb *redis.Client,
	redsync *redsync.Redsync,
	cg *cache.VersionCacheGroup,
	verComparator *vercomp.VersionComparator,
) *HandlerSet {
	panic(wire.Build(
		provider.RepoSet,
		provider.LogicSet,
		provider.HandlerSet,
		wire.Struct(new(HandlerSet), "*"),
	))
}
