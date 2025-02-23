package logic

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/bytedance/sonic"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

type logger struct {
	lg *zap.SugaredLogger
}

func (l logger) Debug(args ...any) {
	l.lg.Debug(append([]any{"asynq: "}, args...)...)
}

func (l logger) Info(args ...any) {
	l.lg.Info(append([]any{"asynq: "}, args...)...)
}

func (l logger) Warn(args ...any) {
	l.lg.Warn(append([]any{"asynq: "}, args...)...)
}

func (l logger) Error(args ...any) {
	l.lg.Error(append([]any{"asynq: "}, args...)...)
}

func (l logger) Fatal(args ...any) {
	l.lg.Fatal(append([]any{"asynq: "}, args...)...)
}

func InitAsynqServer(l *zap.Logger, v *VersionLogic) *asynq.Server {
	var (
		conf = config.GConfig
	)
	server := asynq.NewServer(asynq.RedisClientOpt{
		Addr: conf.Redis.Addr,
		DB:   conf.Redis.AsynqDB,
	}, asynq.Config{
		Logger:      logger{l.Sugar()},
		Concurrency: 100,
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(DiffTask, func(ctx context.Context, task *asynq.Task) error {
		var payload model.PatchTaskPayload
		if err := sonic.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		l.Sugar().Info("generate incremental update package task: ", string(task.Payload()))
		err := v.GenerateIncrementalPackage(ctx, payload.TargetVersionId, payload.CurrentVersionId, payload.OS, payload.Arch)
		if err != nil {
			l.Sugar().Error("generate incremental update package task failed: ", string(task.Payload()))
			return err
		}
		l.Info("generate incremental update package task success",
			zap.Int("current", payload.CurrentVersionId),
			zap.Int("target", payload.TargetVersionId),
			zap.String("os", payload.OS),
			zap.String("arch", payload.Arch),
		)
		return nil
	})

	if err := server.Start(mux); err != nil {
		panic(err)
	}
	return server
}
