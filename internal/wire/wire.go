//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/handler"
	"github.com/MirrorChyan/resource-backend/internal/logic"
	"github.com/MirrorChyan/resource-backend/internal/pkg/vercomp"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/MirrorChyan/resource-backend/internal/tasks"
	"github.com/go-redsync/redsync/v4"
	"github.com/google/wire"
	"github.com/jmoiron/sqlx"
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

var GlobalSet = wire.NewSet(
	repo.Provider,
	logic.Provider,
)

func NewHandlerSet(
	*zap.Logger,
	*ent.Client, *sqlx.DB,
	*redis.Client, *redsync.Redsync, *tasks.TaskQueue,
	*cache.MultiCacheGroup,
	*vercomp.VersionComparator,
) *HandlerSet {
	panic(wire.Build(
		GlobalSet,
		handler.Provider,
		wire.Struct(new(HandlerSet), "*"),
	))
}
