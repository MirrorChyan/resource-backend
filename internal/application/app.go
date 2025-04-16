package application

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/pkg/shutdown"
	"go.uber.org/zap"
	"time"
)

type Adapter interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type App struct {
	adapters        []Adapter
	shutdownTimeout time.Duration
}

func New() *App {
	return &App{
		shutdownTimeout: 5 * time.Second,
	}
}

func (a *App) AddAdapter(adapters ...Adapter) {
	a.adapters = append(a.adapters, adapters...)
}

func (a *App) WithShutdownTimeout(timeout time.Duration) {
	a.shutdownTimeout = timeout
}

func (a *App) Run(ctx context.Context) {
	for _, adapter := range a.adapters {
		go func(adapter Adapter) {
			if err := adapter.Start(ctx); err != nil {
				zap.L().Fatal("adapter start failed", zap.Error(err))
			}
		}(adapter)
	}

	shutdown.GracefulStop(func() {
		a.stop(ctx)
	})
}

func (a *App) stop(ctx context.Context) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, a.shutdownTimeout)
	defer cancel()

	zap.L().Info("shutting down...")

	errCh := make(chan error, len(a.adapters))

	for _, adapter := range a.adapters {
		go func(adapter Adapter) {
			errCh <- adapter.Stop(ctxWithTimeout)
		}(adapter)
	}

	for i := 0; i < len(a.adapters); i++ {
		if err := <-errCh; err != nil {
			go func(err error) {
				zap.L().Error("shutdown failed", zap.Error(err))
			}(err)
		}
	}

	zap.L().Info("graceful stopped")
}
