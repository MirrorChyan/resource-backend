package logic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MirrorChyan/resource-backend/internal/cache"
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

			inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: l.rdb.Options().Addr})
			t.Cleanup(func() { _ = inspector.Close() })
			queued, err := inspector.ListPendingTasks("default")
			require.NoError(t, err)
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
	actual, err := l.initUploadUpdateType(ctx, "res", "android", "arm64", inferred)
	require.NoError(t, err)
	require.Equal(t, types.UpdateIncremental, actual)
	require.Equal(t, 1, client.ResourcePlatformConfig.Query().CountX(ctx))

	// A different arch or os still gets its own first-upload selection.
	for _, p := range []model.Platform{{OS: "android", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}, {OS: "", Arch: ""}} {
		got, initialize, err := l.resolveUploadUpdateType(ctx, "res", p.OS, p.Arch, "app.apk")
		require.NoError(t, err)
		require.True(t, initialize)
		require.Equal(t, types.UpdateFull, got)
	}
}
