package logic

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archiver"
)

// Exercise signing, callback, and the storage worker together so the inferred mode
// controls both filenames and whether file hashes are collected for future patches.
func TestUploadPlatformUpdateType(t *testing.T) {
	for _, c := range []struct {
		name, filename string
		def, own, want types.Update
		history        bool
		wantName       string
	}{
		{"new zip", "app.zip", types.UpdateFull, "", types.UpdateIncremental, false, "resource.zip"},
		{"new uppercase zip", "app.ZIP", types.UpdateFull, "", types.UpdateIncremental, false, "resource.zip"},
		{"new tar.gz", "app.TAR.GZ", types.UpdateFull, "", types.UpdateIncremental, false, "resource.tar.gz"},
		{"new apk", "app.apk", types.UpdateIncremental, "", types.UpdateFull, false, "app.apk"},
		{"new exe", "app.exe", types.UpdateIncremental, "", types.UpdateFull, false, "app.exe"},
		{"new dmg", "app.dmg", types.UpdateIncremental, "", types.UpdateFull, false, "app.dmg"},
		{"unsupported 7z", "app.7z", types.UpdateIncremental, "", types.UpdateFull, false, "app.7z"},
		{"unsupported tar", "app.tar", types.UpdateIncremental, "", types.UpdateFull, false, "app.tar"},
		{"unsupported tgz suffix", "app.tgz", types.UpdateIncremental, "", types.UpdateFull, false, "app.tgz"},
		{"unknown suffix", "app.bin", types.UpdateIncremental, "", types.UpdateFull, false, "app.bin"},
		{"explicit full", "app.zip", types.UpdateIncremental, types.UpdateFull, types.UpdateFull, false, "app.zip"},
		{"explicit incremental", "app.zip", types.UpdateFull, types.UpdateIncremental, types.UpdateIncremental, false, "resource.zip"},
		{"prior full upload", "app.zip", types.UpdateFull, "", types.UpdateFull, true, "app.zip"},
		{"prior incremental upload", "app.zip", types.UpdateIncremental, "", types.UpdateIncremental, true, "resource.zip"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			l, client := newPatchTestLogic(t)
			l.sync = redsync.New(goredis.NewPool(l.rdb))
			l.cacheGroup.MultiVersionInfoCache = cache.NewCache[string, *model.MultiVersionInfo](-1)
			res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").
				SetUpdateType(c.def.String()).SaveX(ctx)
			if c.own != "" {
				_, err := l.resourceLogic.SetPlatformUpdateType(ctx, res.ID, "android", "arm64", c.own)
				require.NoError(t, err)
			}
			if c.history {
				// A purged package in beta still counts as a previous upload for stable.
				old := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).
					SetChannel(version.ChannelBeta).SaveX(ctx)
				client.Storage.Create().SetVersion(old).SetOs("android").SetArch("arm64").
					SetUpdateType(storage.UpdateTypeFull).SaveX(ctx)
			}
			// Prime the serving cache with the old/default setting before initialization.
			_, err := l.resourceLogic.ResolveUpdateType(ctx, res.ID, "android", "arm64")
			require.NoError(t, err)
			before := client.ResourcePlatformConfig.Query().CountX(ctx)
			param := model.CreateVersionParam{
				ResourceID: res.ID, Name: "v2", OS: "android", Arch: "arm64", Channel: "stable", Filename: c.filename,
			}
			token, err := l.CreatePreSignedUrl(ctx, param)
			require.NoError(t, err)
			require.Equal(t, c.wantName, token.Name)
			require.Equal(t, before, client.ResourcePlatformConfig.Query().CountX(ctx), "signing alone must not pin the mode")

			source := filepath.Join(l.storageLogic.OSSDir, token.Key)
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
			files := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(files, "app.txt"), []byte("content"), 0o644))
			switch types.IncrementalFileType(c.filename) {
			case types.Zip:
				require.NoError(t, archiver.CompressToZip(files, source))
			case types.Tgz:
				require.NoError(t, archiver.CompressToTarGz(files, source))
			default:
				require.NoError(t, os.WriteFile(source, []byte("installer"), 0o644))
			}
			statusKey, err := l.ProcessCreateVersionCallback(ctx, model.CreateVersionCallBackParam{
				ResourceID: res.ID, Name: param.Name, OS: param.OS, Arch: param.Arch, Channel: param.Channel, Key: token.Key,
			})
			require.NoError(t, err)
			require.NotEmpty(t, statusKey)
			got, err := l.resourceLogic.ResolveUpdateType(ctx, res.ID, param.OS, param.Arch)
			require.NoError(t, err)
			require.Equal(t, c.want, got)
			if c.own == "" && !c.history {
				require.Equal(t, before+1, client.ResourcePlatformConfig.Query().CountX(ctx))
			} else {
				require.Equal(t, before, client.ResourcePlatformConfig.Query().CountX(ctx))
			}

			queued := pendingUploads(t, l)
			require.Len(t, queued, 1)
			var payload model.StorageInfoCreatePayload
			require.NoError(t, sonic.Unmarshal(queued[0].Payload, &payload))
			if c.want == types.UpdateIncremental {
				require.Equal(t, types.IncrementalFileType(c.filename), payload.IncrementalType)
			} else {
				require.Empty(t, payload.IncrementalType)
			}
			require.NoError(t, doHandleCalculatePackageHash(l.logger, l)(ctx, asynq.NewTask(queued[0].Type, queued[0].Payload)))
			stored, err := l.storageLogic.GetFullUpdateStorage(ctx, payload.VersionId, param.OS, param.Arch)
			require.NoError(t, err)
			if c.want == types.UpdateIncremental {
				require.Contains(t, stored.FileHashes, "app.txt")
			} else {
				require.Empty(t, stored.FileHashes)
				require.Equal(t, source, stored.PackagePath)
			}
			status, err := l.GetProcessingStatus(ctx, statusKey)
			require.NoError(t, err)
			require.Equal(t, misc.StatusCompleted, status)

			// Later filenames never flip the mode of this platform.
			param.Name, param.Filename = "v3", "other.exe"
			_, err = l.CreatePreSignedUrl(ctx, param)
			if c.want == types.UpdateIncremental {
				require.ErrorContains(t, err, "file ext not supported")
			} else {
				require.NoError(t, err)
				param.Filename = "other.zip"
				next, err := l.CreatePreSignedUrl(ctx, param)
				require.NoError(t, err)
				require.Equal(t, "other.zip", next.Name)
			}
		})
	}
}

func pendingUploads(t *testing.T, l *VersionLogic) []*asynq.TaskInfo {
	t.Helper()
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: l.rdb.Options().Addr})
	t.Cleanup(func() { _ = inspector.Close() })
	queued, err := inspector.ListPendingTasks("default")
	require.NoError(t, err)
	return queued
}

func TestQueuedUploadsKeepAcceptedMode(t *testing.T) {
	ctx := context.Background()
	l, client := newPatchTestLogic(t)
	l.cacheGroup.MultiVersionInfoCache = cache.NewCache[string, *model.MultiVersionInfo](-1)
	l.cacheGroup.FullUpdateStorageCache = cache.NewCache[string, *ent.Storage](-1)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	first := client.Version.Create().SetResourceID("res").SetName("v1").SetNumber(1).SaveX(ctx)
	second := client.Version.Create().SetResourceID("res").SetName("v2").SetNumber(2).SaveX(ctx)
	require.NoError(t, os.MkdirAll(l.storageLogic.OSSDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(l.storageLogic.OSSDir, "app.apk"), []byte("installer"), 0o644))
	param := model.CreateVersionCallBackParam{ResourceID: "res", Name: "v1", OS: "android", Arch: "arm64", Channel: "stable", Key: "app.apk"}
	_, err := l.ProcessCreateVersionCallback(ctx, param)
	require.NoError(t, err)
	require.Zero(t, client.Storage.Query().CountX(ctx), "the worker has not run yet")
	mode, initialize, err := l.resourceLogic.resolveUploadUpdateType(ctx, "res", param.OS, param.Arch, "next.zip")
	require.NoError(t, err)
	require.False(t, initialize)
	require.Equal(t, types.UpdateFull, mode, "a queued upload already determines the platform mode")

	// The second upload must keep full processing, even before the first worker runs.
	require.NoError(t, os.WriteFile(filepath.Join(l.storageLogic.OSSDir, "next.zip"), []byte("opaque full package"), 0o644))
	param.Name, param.Key = "v2", "next.zip"
	_, err = l.ProcessCreateVersionCallback(ctx, param)
	require.NoError(t, err)
	queued := pendingUploads(t, l)
	require.Len(t, queued, 2)
	// Changing the platform now controls future callbacks and serving, not the format
	// of already accepted packages (an APK cannot be retroactively made incremental).
	_, err = l.resourceLogic.SetPlatformUpdateType(ctx, "res", param.OS, param.Arch, types.UpdateIncremental)
	require.NoError(t, err)
	for _, task := range queued {
		var payload model.StorageInfoCreatePayload
		require.NoError(t, sonic.Unmarshal(task.Payload, &payload))
		require.Empty(t, payload.IncrementalType)
		require.NoError(t, doHandleCalculatePackageHash(l.logger, l)(ctx, asynq.NewTask(task.Type, task.Payload)))
	}
	// The existing missing-hash guard skips patches and serves full packages safely.
	target, err := l.storageLogic.GetFullUpdateStorage(ctx, second.ID, param.OS, param.Arch)
	require.NoError(t, err)
	require.Empty(t, target.FileHashes)
	require.NoError(t, l.GenerateIncrementalPackage(ctx, "res", second.ID, first.ID, param.OS, param.Arch))
	got, err := l.doProcessUpdateRequest(ctx, model.UpdateRequestParam{
		ResourceId: "res", CurrentVersionName: "v1",
		TargetVersionInfo: &model.LatestVersionInfo{
			VersionId: second.ID, OS: param.OS, Arch: param.Arch,
			PackagePath: sql.NullString{String: target.PackagePath, Valid: true},
		},
	})
	require.NoError(t, err)
	require.Equal(t, types.UpdateFull.String(), got.UpdateType)
	require.Equal(t, target.PackagePath, got.PackagePath)
}

func TestOnlyPatchHistoryStillCountsAsPriorUpload(t *testing.T) {
	ctx := context.Background()
	l, client, _ := newResourceTestLogic(t)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	v1 := client.Version.Create().SetResourceID("res").SetName("v1").SetNumber(1).SaveX(ctx)
	v2 := client.Version.Create().SetResourceID("res").SetName("v2").SetNumber(2).SaveX(ctx)
	client.Storage.Create().SetVersion(v2).SetOldVersion(v1).SetOs("android").SetArch("arm64").
		SetUpdateType(storage.UpdateTypeIncremental).SaveX(ctx)
	got, initialize, err := l.resolveUploadUpdateType(ctx, "res", "android", "arm64", "app.apk")
	require.NoError(t, err)
	require.False(t, initialize)
	require.Equal(t, types.UpdateIncremental, got)
	// History is scoped to the resource and the exact os/arch tuple.
	client.Resource.Create().SetID("other").SetName("other").SetDescription("").SaveX(ctx)
	got, initialize, err = l.resolveUploadUpdateType(ctx, "other", "android", "arm64", "app.apk")
	require.NoError(t, err)
	require.True(t, initialize)
	require.Equal(t, types.UpdateFull, got)
}

// Fail just the status SET while allowing the pending marker and cleanup to work.
type failStatusWrite struct{ enabled bool }

func (h *failStatusWrite) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *failStatusWrite) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *failStatusWrite) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.enabled && cmd.Name() == "set" {
			key, _ := cmd.Args()[1].(string)
			if strings.HasPrefix(key, misc.StatusPollingPrefix+":") {
				return errors.New("injected status write failure")
			}
		}
		return next(ctx, cmd)
	}
}

func TestFailedSubmissionDoesNotPinPlatform(t *testing.T) {
	for _, failure := range []string{"status write", "enqueue"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			l, client := newPatchTestLogic(t)
			client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
			client.Version.Create().SetResourceID("res").SetName("v1").SetNumber(1).SaveX(ctx)
			files := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(files, "app.txt"), []byte("data"), 0o644))
			require.NoError(t, os.MkdirAll(l.storageLogic.OSSDir, 0o755))
			require.NoError(t, archiver.CompressToZip(files, filepath.Join(l.storageLogic.OSSDir, "app.zip")))
			hook := &failStatusWrite{enabled: failure == "status write"}
			l.rdb.AddHook(hook)
			if failure == "enqueue" {
				require.NoError(t, l.taskQueue.Close())
			}
			param := model.CreateVersionCallBackParam{ResourceID: "res", Name: "v1", OS: "android", Arch: "arm64", Channel: "stable", Key: "app.zip"}
			_, err := l.ProcessCreateVersionCallback(ctx, param)
			require.Error(t, err)
			require.Zero(t, client.ResourcePlatformConfig.Query().CountX(ctx))
			keys, err := l.rdb.Keys(ctx, "*").Result()
			require.NoError(t, err)
			require.Empty(t, keys, "failed submissions must remove pending/polling markers")

			hook.enabled = false
			if failure == "enqueue" {
				queue := asynq.NewClient(asynq.RedisClientOpt{Addr: l.rdb.Options().Addr})
				t.Cleanup(func() { _ = queue.Close() })
				l.taskQueue.Client = queue
			}
			// A different-format retry is free to choose full mode.
			require.NoError(t, os.WriteFile(filepath.Join(l.storageLogic.OSSDir, "app.apk"), []byte("installer"), 0o644))
			param.Key = "app.apk"
			_, err = l.ProcessCreateVersionCallback(ctx, param)
			require.NoError(t, err)
			got, err := l.resourceLogic.ResolveUpdateType(ctx, "res", param.OS, param.Arch)
			require.NoError(t, err)
			require.Equal(t, types.UpdateFull, got)
		})
	}
}

func TestInvalidFirstArchiveDoesNotPinPlatform(t *testing.T) {
	ctx := context.Background()
	l, client := newPatchTestLogic(t)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	client.Version.Create().SetResourceID("res").SetName("v1").SetNumber(1).SaveX(ctx)
	source := filepath.Join(l.storageLogic.OSSDir, "bad.zip")
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	require.NoError(t, os.WriteFile(source, []byte("not a zip"), 0o644))
	param := model.CreateVersionCallBackParam{ResourceID: "res", Name: "v1", OS: "android", Arch: "arm64", Channel: "stable", Key: "bad.zip"}
	_, err := l.ProcessCreateVersionCallback(ctx, param)
	require.ErrorIs(t, err, misc.NotAllowedFileTypeError)
	require.Zero(t, client.ResourcePlatformConfig.Query().CountX(ctx))

	// A retry with an installer is free to select full mode for the same platform.
	require.NoError(t, os.Rename(source, filepath.Join(l.storageLogic.OSSDir, "app.apk")))
	param.Key = "app.apk"
	_, err = l.ProcessCreateVersionCallback(ctx, param)
	require.NoError(t, err)
	got, err := l.resourceLogic.ResolveUpdateType(ctx, "res", param.OS, param.Arch)
	require.NoError(t, err)
	require.Equal(t, types.UpdateFull, got)
}

func TestUploadInitializationPreservesConcurrentSetting(t *testing.T) {
	ctx := context.Background()
	l, client, _ := newResourceTestLogic(t)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	inferred, initialize, err := l.resolveUploadUpdateType(ctx, "res", "android", "arm64", "app.apk")
	require.NoError(t, err)
	require.True(t, initialize)
	require.Equal(t, types.UpdateFull, inferred)

	// An admin or another upload sets the platform after inference, before saving.
	_, err = l.SetPlatformUpdateType(ctx, "res", "android", "arm64", types.UpdateIncremental)
	require.NoError(t, err)
	actual, err := l.initUploadUpdateType(ctx, "res", "android", "arm64", inferred, func(actual types.Update) error {
		require.Equal(t, types.UpdateIncremental, actual)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, types.UpdateIncremental, actual)
	require.Equal(t, 1, client.ResourcePlatformConfig.Query().CountX(ctx))
	failure := errors.New("submission rejected")
	_, err = l.initUploadUpdateType(ctx, "res", "android", "arm64", inferred, func(actual types.Update) error {
		return failure
	})
	require.ErrorIs(t, err, failure)
	actual, err = l.ResolveUpdateType(ctx, "res", "android", "arm64")
	require.NoError(t, err)
	require.Equal(t, types.UpdateIncremental, actual, "rollback must preserve an existing admin setting")

	// A different arch or os still gets its own first-upload selection.
	for _, p := range []model.Platform{{OS: "android", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}, {OS: "", Arch: ""}} {
		got, initialize, err := l.resolveUploadUpdateType(ctx, "res", p.OS, p.Arch, "app.apk")
		require.NoError(t, err)
		require.True(t, initialize)
		require.Equal(t, types.UpdateFull, got)
	}
}
