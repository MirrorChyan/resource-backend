package logic

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/bytedance/sonic"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"time"
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
	mux.HandleFunc(misc.DiffTask, doHandleGeneratePackage(l, v))
	mux.HandleFunc(misc.ProcessStorageTask, doHandleCalculatePackageHash(l, v))

	if err := server.Start(mux); err != nil {
		panic(err)
	}
	return server
}

func doHandleCalculatePackageHash(l *zap.Logger, v *VersionLogic) func(ctx context.Context, task *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		c, ok := asynq.GetRetryCount(ctx)
		if ok {
			l.Info("retry count", zap.Int("count", c))
		}

		var payload model.StorageInfoCreatePayload
		if err := sonic.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}

		var (
			dest        = payload.Dest
			system      = payload.OS
			arch        = payload.Arch
			resourceId  = payload.ResourceId
			versionId   = payload.VersionId
			channel     = payload.Channel
			hashes      = payload.FileHashes
			versionName = payload.VersionName
		)

		err := v.DoProcessStorage(ctx,
			resourceId,
			versionId, versionName,
			channel, system, arch, dest,
			hashes,
		)
		if err != nil {
			l.Error("failed to CreateFullUpdateStorage",
				zap.Error(err),
				zap.String("resource id", resourceId),
				zap.Int("version id", versionId),
				zap.String("version name", versionName),
				zap.String("channel", channel),
				zap.String("os", system),
				zap.String("arch", arch),
			)
			return err
		}

		// delete pending key
		mk := strings.Join([]string{misc.ProcessStoragePendingKey,
			resourceId, strconv.Itoa(versionId), channel, system, arch,
		}, ":")
		v.rdb.Del(ctx, mk)

		return nil
	}
}

func doHandleGeneratePackage(l *zap.Logger, v *VersionLogic) func(ctx context.Context, task *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		var payload model.PatchTaskPayload
		if err := sonic.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}
		l.Sugar().Info("generate incremental update package task: ", string(task.Payload()))
		var (
			start = time.Now()

			target     = payload.TargetVersionId
			current    = payload.CurrentVersionId
			system     = payload.OS
			arch       = payload.Arch
			resourceId = payload.ResourceId
		)
		err := v.GenerateIncrementalPackage(ctx, resourceId, target, current, system, arch)
		if err != nil {
			l.Sugar().Error("generate incremental update package task failed: ", string(task.Payload()))
			return err
		}
		l.Info("generate incremental update package task success",
			zap.Int64("cost time", time.Since(start).Milliseconds()),
			zap.String("resource id", resourceId),
			zap.Int("current", current),
			zap.Int("target", target),
			zap.String("os", system),
			zap.String("arch", arch),
		)
		return nil
	}
}
