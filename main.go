package main

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/application"
	"github.com/MirrorChyan/resource-backend/internal/interfaces/rest"
	"github.com/MirrorChyan/resource-backend/internal/pkg/restserver"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/db"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	_ "github.com/MirrorChyan/resource-backend/internal/pkg/banner"
	"github.com/MirrorChyan/resource-backend/internal/pkg/logger"
	"github.com/MirrorChyan/resource-backend/internal/pkg/vercomp"
	"github.com/MirrorChyan/resource-backend/internal/tasks"
	"github.com/MirrorChyan/resource-backend/internal/wire"
	"go.uber.org/zap"
)

func main() {

	setUpConfigAndLog()

	mysql, dx, err := db.LoadDataSource()
	if err != nil {
		zap.L().Fatal("failed to connect to database",
			zap.Error(err),
		)
	}

	defer func(m *ent.Client) {
		if err := m.Close(); err != nil {
			zap.L().Fatal("failed to close database")
		}
	}(mysql)

	if err := mysql.Schema.Create(context.Background()); err != nil {
		zap.L().Fatal("failed creating schema resources",
			zap.Error(err),
		)
	}

	// deps
	var (
		redis      = db.NewRedis()
		queue      = tasks.NewTaskQueue()
		dl         = db.NewRedSync(redis)
		group      = cache.NewVersionCacheGroup(redis)
		comparator = vercomp.NewComparator()
		app        = application.New()
	)

	handlerSet := wire.NewHandlerSet(zap.L(),
		mysql, dx,
		redis, dl, queue,
		group,
		comparator)

	restSrv := rest.NewRouter()
	rest.InitRoutes(restSrv, handlerSet)

	app.AddAdapter(
		restserver.NewAdapter(restSrv),
	)

	app.Run(context.Background())
}

func setUpConfigAndLog() {
	// in the full life cycle
	config.InitGlobalConfig()
	zap.ReplaceGlobals(logger.New())
}
