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
	mux.HandleFunc(misc.PurgeTask, doHandlePurge(l, v))

	if err := server.Start(mux); err != nil {
		panic(err)
	}

	initScheduler(l)

	return server
}

func initScheduler(l *zap.Logger) {
	var (
		conf = config.GConfig
	)

	location, _ := time.LoadLocation("Asia/Shanghai")

	scheduler := asynq.NewScheduler(asynq.RedisClientOpt{
		Addr: conf.Redis.Addr,
		DB:   conf.Redis.AsynqDB,
	}, &asynq.SchedulerOpts{

		HeartbeatInterval: time.Minute,
		Logger:            logger{l.Sugar()},
		Location:          location,
		PostEnqueueFunc: func(info *asynq.TaskInfo, err error) {
			l.Info("scheduler enqueued a task",
				zap.String("task id", info.ID),
				zap.Any("enqueued at", info),
				zap.Error(err),
			)
		},
	})
	l.Info("scheduler starting",
		zap.String("location", location.String()),
	)
	id, err := scheduler.Register("0 5 * * ?", asynq.NewTask(misc.PurgeTask, nil))
	if err != nil {
		l.Error("failed to register scheduler",
			zap.Error(err),
		)
		panic(err)
	}
	l.Info("scheduler registered",
		zap.String("id", id),
	)

	if err := scheduler.Start(); err != nil {
		panic(err)
	}

}

func doHandlePurge(l *zap.Logger, v *VersionLogic) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		l.Warn("start purge old storages")
		err := v.storageLogic.ClearOldStorages(ctx)
		if err != nil {
			l.Error("failed to purge old storages",
				zap.Error(err),
			)
		}
		l.Warn("end purge old storages")
		// ignore error
		return nil
	}
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
