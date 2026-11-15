package logic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/config"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archiver"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"github.com/MirrorChyan/resource-backend/internal/pkg/gitpack"
	"github.com/bytedance/sonic"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// releasePendingKey deletes the pending key of a version only while it holds the status
// key of the upload: once it expires, another upload may hold it.
var releasePendingKey = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

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
		cnt, _ := asynq.GetRetryCount(ctx)
		l.Warn("start purge old storages with", zap.Int("retry cnt", cnt))
		err := v.storageLogic.ClearOldStorages(ctx)
		if err != nil {
			l.Error("failed to purge old storages",
				zap.Error(err),
			)
			return err
		}
		l.Warn("end purge old storages")
		return nil
	}
}

func doHandleCalculatePackageHash(l *zap.Logger, v *VersionLogic) func(ctx context.Context, task *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) (retErr error) {
		c, ok := asynq.GetRetryCount(ctx)
		if ok {
			l.Info("retry count", zap.Int("count", c))
		}

		var payload model.StorageInfoCreatePayload
		if err := sonic.Unmarshal(task.Payload(), &payload); err != nil {
			return err
		}

		var (
			source      = payload.Source
			system      = payload.OS
			arch        = payload.Arch
			resourceId  = payload.ResourceId
			versionId   = payload.VersionId
			channel     = payload.Channel
			versionName = payload.VersionName
			fileType    = payload.IncrementalType
			statusKey   = payload.StatusKey
		)

		mk := strings.Join([]string{misc.ProcessStoragePendingKey,
			resourceId, strconv.Itoa(versionId), channel, system, arch,
		}, ":")
		defer func() {
			if retErr == nil || statusKey == "" {
				return
			}
			maxRetry, _ := asynq.GetMaxRetry(ctx)
			retryCount, _ := asynq.GetRetryCount(ctx)
			// a run that skips its retries is the last one too
			if retryCount < maxRetry && !errors.Is(retErr, asynq.SkipRetry) {
				return
			}
			c := context.WithoutCancel(ctx)
			v.rdb.Set(c, misc.StatusPollingKey(statusKey), int(misc.StatusFailed), time.Minute*30)
			// free the version and platform for another upload
			if err := releasePendingKey.Run(c, v.rdb, []string{mk}, statusKey).Err(); err != nil {
				l.Warn("failed to release the pending key of a failed upload",
					zap.String("key", mk),
					zap.Error(err),
				)
			}
		}()

		// an admin may have moved the platform into or out of pack since the upload was
		// queued, and the package would be served as what it is not
		ut, _, err := v.resourceLogic.resolveUploadUpdateType(ctx, resourceId, system, arch, source)
		if err != nil {
			return err
		}
		if (ut == types.UpdatePack) != (fileType == types.Pack) {
			l.Warn("upload rejected, the update type of its platform changed",
				zap.String("resource id", resourceId),
				zap.String("version name", versionName),
				zap.String("update type", ut.String()),
				zap.String("file type", string(fileType)),
			)
			return fmt.Errorf("the platform serves %s updates now, not this upload: %w", ut, asynq.SkipRetry)
		}

		var (
			dest   string
			hashes map[string]string
		)

		if fileType == types.Pack {
			err := v.DoProcessPackStorage(ctx, resourceId, versionId, versionName, channel, system, arch, source)
			if errors.Is(err, gitpack.ErrInvalid) {
				l.Warn("pack snapshot rejected",
					zap.String("resource id", resourceId),
					zap.String("version name", versionName),
					zap.Error(err),
				)
				// trying again cannot help
				return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
			}
			if err != nil {
				return err
			}
			v.rdb.Del(ctx, mk)
			if statusKey != "" {
				v.rdb.Set(ctx, misc.StatusPollingKey(statusKey), int(misc.StatusCompleted), time.Minute*30)
			}
			return nil
		}

		if fileType != "" {
			// Incremental: copy source to local storage, unpack, compute file hashes
			destDir := v.storageLogic.BuildVersionStorageDirPath(resourceId, versionId, system, arch)
			if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
				return err
			}

			filename := misc.DefaultResourceName + types.GetFileSuffix(fileType)
			dest = filepath.Join(destDir, filename)

			if err := fileops.CopyFile(source, dest); err != nil {
				l.Error("failed to copy source to local storage",
					zap.String("source", source),
					zap.String("dest", dest),
					zap.Error(err),
				)
				return err
			}

			extractDir := v.storageLogic.BuildVersionResourceStorageDirPath(resourceId, versionId, system, arch)
			// clear storage directory
			defer func() {
				go func() {
					l.Warn("clean storage directory",
						zap.String("path", extractDir),
					)
					if e := os.RemoveAll(extractDir); e != nil {
						l.Error("Failed to remove storage directory",
							zap.Error(e),
						)
					}
				}()
			}()
			switch fileType {
			case types.Zip:
				if err := archiver.UnpackZip(dest, extractDir); err != nil {
					l.Error("failed to unpack zip",
						zap.String("dest", dest),
						zap.Error(err),
					)
					return err
				}
			case types.Tgz:
				if err := archiver.UnpackTarGz(dest, extractDir); err != nil {
					l.Error("failed to unpack tgz",
						zap.String("dest", dest),
						zap.Error(err),
					)
					return err
				}
			}

			hashes, err = filehash.GetAll(extractDir)
			if err != nil {
				l.Error("failed to calculate file hashes",
					zap.String("extract dir", extractDir),
					zap.Error(err),
				)
				return err
			}
		} else {
			// Full update: use source directly
			dest = source
		}

		err = v.DoProcessStorage(ctx,
			resourceId,
			versionId, versionName,
			channel, system, arch, dest,
			fileType,
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

		// Delete the process pending key
		v.rdb.Del(ctx, mk)

		// Update status polling key to completed
		if statusKey != "" {
			v.rdb.Set(ctx, misc.StatusPollingKey(statusKey), int(misc.StatusCompleted), time.Minute*30)
		}

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
