package logic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/MirrorChyan/resource-backend/internal/pkg/gitpack"
	"github.com/MirrorChyan/resource-backend/internal/pkg/vercomp"
	"github.com/MirrorChyan/resource-backend/internal/repo"
)

// packRepo is a git repository whose commits are uploaded as snapshots, the way CI does.
type packRepo struct {
	t   *testing.T
	dir string
}

func newPackRepo(t *testing.T) *packRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := &packRepo{t: t, dir: t.TempDir()}
	r.git(nil, "", "init", "-q")
	return r
}

func (r *packRepo) git(stdin []byte, date string, args ...string) []byte {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = r.dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if date == "" {
		date = "2026-11-01T10:00:00+08:00"
	}
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.com", "GIT_COMMITTER_DATE="+date,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(r.t, err, "git %v: %s", args, stderr.String())
	return out
}

// commit commits the files at the committer date, and returns the commit, its version
// name and its snapshot.
func (r *packRepo) commit(date string, files map[string]string) (commit, name string, snapshot []byte) {
	r.t.Helper()
	for path, content := range files {
		full := filepath.Join(r.dir, path)
		require.NoError(r.t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(r.t, os.WriteFile(full, []byte(content), 0o644))
	}
	r.git(nil, date, "add", "-A")
	r.git(nil, date, "commit", "-q", "--allow-empty", "-m", "commit at "+date)
	commit = strings.TrimSpace(string(r.git(nil, "", "rev-parse", "HEAD")))
	at, err := time.Parse(time.RFC3339, date)
	require.NoError(r.t, err)
	objects := r.git(nil, "", "rev-list", "--objects", "--no-walk", commit)
	return commit, at.UTC().Format(gitpack.VersionNameLayout), r.git(objects, "", "pack-objects", "-q", "--stdout")
}

// newPackTestLogic sets up the resource "res" with the update type ut.
func newPackTestLogic(t *testing.T, ut types.Update) (*VersionLogic, *ent.Client) {
	t.Helper()
	l, client := newPatchTestLogic(t)
	l.rawQuery = repo.NewRawQuery(l.repo)
	l.storageLogic.rawQuery = l.rawQuery
	l.comparator = vercomp.NewComparator()
	l.cacheGroup.MultiVersionInfoCache = cache.NewCache[string, *model.MultiVersionInfo](-1, 1024)
	// caching for real: a republished version must not be served from a stale entry
	l.cacheGroup.FullUpdateStorageCache = cache.NewCache[string, *model.FullStorage](time.Hour, 1024)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").
		SetUpdateType(ut.String()).SaveX(context.Background())
	return l, client
}

// uploadPack runs an upload to stable through signing, the callback and the storage task.
func uploadPack(t *testing.T, l *VersionLogic, name string, snapshot []byte) (misc.PollingStatus, error) {
	t.Helper()
	return uploadPackTo(t, l, name, "stable", snapshot)
}

func uploadPackTo(t *testing.T, l *VersionLogic, name, channel string, snapshot []byte) (misc.PollingStatus, error) {
	t.Helper()
	ctx := context.Background()
	statusKey, task, err := acceptPack(t, l, name, channel, snapshot)
	if err != nil {
		return 0, err
	}
	taskErr := doHandleCalculatePackageHash(l.logger, l)(ctx, task)
	status, err := l.GetProcessingStatus(ctx, statusKey)
	require.NoError(t, err)
	return status, taskErr
}

// acceptPack runs an upload through signing and the callback, and returns the storage
// task it queued.
func acceptPack(t *testing.T, l *VersionLogic, name, channel string, snapshot []byte) (string, *asynq.Task, error) {
	t.Helper()
	ctx := context.Background()
	token, err := l.CreatePreSignedUrl(ctx, model.CreateVersionParam{
		ResourceID: "res", Name: name, Channel: channel, Filename: "res-" + name + ".pack",
	})
	if err != nil {
		return "", nil, err
	}
	require.Equal(t, "snapshot.pack", token.Name)
	source := filepath.Join(l.storageLogic.OSSDir, token.Key)
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	require.NoError(t, os.WriteFile(source, snapshot, 0o644))

	statusKey, err := l.ProcessCreateVersionCallback(ctx, model.CreateVersionCallBackParam{
		ResourceID: "res", Name: name, Channel: channel, Key: token.Key,
	})
	require.NoError(t, err)
	require.NotEmpty(t, statusKey, "a pack resource processes every upload")

	queued := pendingUploads(t, l)
	require.NotEmpty(t, queued)
	last := queued[len(queued)-1]
	return statusKey, asynq.NewTask(last.Type, last.Payload), nil
}

func latestName(t *testing.T, l *VersionLogic) string {
	t.Helper()
	return latestNameIn(t, l, "stable")
}

func latestNameIn(t *testing.T, l *VersionLogic, channel string) string {
	t.Helper()
	info, err := l.GetMultiLatestVersionInfo("res", "", "", channel)
	require.NoError(t, err)
	return info.VersionName
}

func versionID(t *testing.T, client *ent.Client, name string) int {
	t.Helper()
	return client.Version.Query().Where(version.Name(name)).OnlyX(context.Background()).ID
}

func TestPackUploads(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdatePack)
	r := newPackRepo(t)
	c1, n1, s1 := r.commit("2026-10-31T01:22:49+08:00", map[string]string{"a.txt": "1", "d/b.txt": "x"})
	_, n2, s2 := r.commit("2026-10-31T01:30:00+08:00", map[string]string{"a.txt": "2"})

	status, err := uploadPack(t, l, n1, s1)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)
	// the first upload of a git pack to a pack resource makes the platform a pack platform
	ut, err := l.resourceLogic.ResolveUpdateType(ctx, "res", "", "")
	require.NoError(t, err)
	require.Equal(t, types.UpdatePack, ut)

	stored, err := l.storageLogic.GetFullUpdateStorage(ctx, versionID(t, client, n1), "", "")
	require.NoError(t, err)
	require.Equal(t, c1, stored.PackCommit)
	require.Empty(t, stored.FileHashes)
	hashes, err := l.storageLogic.GetFileHashes(ctx, stored.ID)
	require.NoError(t, err)
	require.Contains(t, hashes, gitpack.RootTreeKey)
	require.Contains(t, hashes, "d/b.txt")
	require.FileExists(t, stored.PackagePath)
	meta, err := gitpack.ReadMeta(stored.PackagePath)
	require.NoError(t, err)
	require.Equal(t, gitpack.Meta{Commit: c1}, meta)

	status, err = uploadPack(t, l, n2, s2)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)
	require.Equal(t, n2, latestName(t, l))

	// uploading n1 again publishes it again: the branch moved back
	status, err = uploadPack(t, l, n1, s1)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)
	require.Equal(t, n1, latestName(t, l))
	require.Equal(t, 1, client.Storage.Query().Where(storage.VersionStorages(versionID(t, client, n1))).CountX(ctx))

	t.Run("name of another commit time", func(t *testing.T) {
		status, err := uploadPack(t, l, "20200101000000", s2)
		require.ErrorIs(t, err, gitpack.ErrInvalid)
		require.ErrorIs(t, err, asynq.SkipRetry)
		require.Equal(t, misc.StatusFailed, status)
		// the version is free for a corrected upload
		_, err = l.CreatePreSignedUrl(ctx, model.CreateVersionParam{
			ResourceID: "res", Name: "20200101000000", Channel: "stable", Filename: "x.pack",
		})
		require.NoError(t, err)
	})

	t.Run("another commit in the same second", func(t *testing.T) {
		_, same, other := r.commit("2026-10-31T01:22:49+08:00", map[string]string{"a.txt": "other"})
		require.Equal(t, n1, same)
		status, err := uploadPack(t, l, n1, other)
		require.ErrorIs(t, err, gitpack.ErrInvalid)
		require.Equal(t, misc.StatusFailed, status)
		require.Equal(t, n1, latestName(t, l))
	})

	t.Run("not a git pack", func(t *testing.T) {
		_, err := l.CreatePreSignedUrl(ctx, model.CreateVersionParam{
			ResourceID: "res", Name: "20300101000000", Channel: "stable", Filename: "app.zip",
		})
		require.ErrorContains(t, err, "file ext not supported")
	})
}

// A version name stays taken on resources other than pack ones.
func TestOtherResourcesKeepNameConflict(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdateIncremental)
	res := client.Resource.Query().OnlyX(ctx)
	v := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)
	client.Storage.Create().SetVersion(v).SetUpdateType(storage.UpdateTypeFull).SaveX(ctx)
	_, err := l.CreatePreSignedUrl(ctx, model.CreateVersionParam{
		ResourceID: "res", Name: "v1", Channel: "stable", Filename: "app.zip",
	})
	require.ErrorIs(t, err, errs.ErrResourceVersionNameConflict)
}

// Before pack resources a .pack file was a plain file to every resource; it stays one
// on resources other than pack ones, even when it is a git pack.
func TestPackFileStaysPlainOnOtherResources(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdateIncremental)
	r := newPackRepo(t)
	_, _, snapshot := r.commit("2026-11-01T10:00:00+08:00", map[string]string{"a.txt": "1"})
	param := model.CreateVersionParam{
		ResourceID: "res", Name: "v1", OS: "windows", Arch: "amd64", Channel: "stable", Filename: "data.pack",
	}
	token, err := l.CreatePreSignedUrl(ctx, param)
	require.NoError(t, err)
	require.Equal(t, "data.pack", token.Name, "a full upload keeps its name")

	source := filepath.Join(l.storageLogic.OSSDir, token.Key)
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	require.NoError(t, os.WriteFile(source, snapshot, 0o644))
	statusKey, err := l.ProcessCreateVersionCallback(ctx, model.CreateVersionCallBackParam{
		ResourceID: "res", Name: "v1", OS: "windows", Arch: "amd64", Channel: "stable", Key: token.Key,
	})
	require.NoError(t, err)
	queued := pendingUploads(t, l)
	require.NoError(t, doHandleCalculatePackageHash(l.logger, l)(ctx, asynq.NewTask(queued[0].Type, queued[0].Payload)))
	status, err := l.GetProcessingStatus(ctx, statusKey)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)

	ut, err := l.resourceLogic.ResolveUpdateType(ctx, "res", "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, types.UpdateFull, ut)
	stored, err := l.storageLogic.GetFullUpdateStorage(ctx, versionID(t, client, "v1"), "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, source, stored.PackagePath, "stored as it is")
	require.Empty(t, stored.PackCommit)
}

// A platform with uploads cannot switch into or out of pack: its stored packages are of
// the other kind. Without uploads, and between full and incremental, it still can.
func TestPackSwitchNeedsAnEmptyPlatform(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdateIncremental)
	res := client.Resource.Query().OnlyX(ctx)
	v := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)
	client.Storage.Create().SetVersion(v).SetOs("android").SetArch("arm64").SetUpdateType(storage.UpdateTypeFull).SaveX(ctx)

	_, err := l.resourceLogic.SetPlatformUpdateType(ctx, "res", "android", "arm64", types.UpdatePack)
	require.ErrorContains(t, err, "cannot switch between pack")
	_, err = l.resourceLogic.SetPlatformUpdateType(ctx, "res", "android", "arm64", types.UpdateFull)
	require.NoError(t, err)
	_, err = l.resourceLogic.SetPlatformUpdateType(ctx, "res", "windows", "amd64", types.UpdatePack)
	require.NoError(t, err, "no uploads yet")

	// out of pack: the platform takes the resource's type back, or another one
	client.Storage.Create().SetVersion(v).SetOs("windows").SetArch("amd64").SetUpdateType(storage.UpdateTypeFull).SaveX(ctx)
	_, err = l.resourceLogic.DeletePlatformUpdateType(ctx, "res", "windows", "amd64")
	require.ErrorContains(t, err, "cannot switch between pack")
	_, err = l.resourceLogic.SetPlatformUpdateType(ctx, "res", "windows", "amd64", types.UpdateFull)
	require.ErrorContains(t, err, "cannot switch between pack")
	_, err = l.resourceLogic.SetPlatformUpdateType(ctx, "res", "windows", "amd64", types.UpdatePack)
	require.NoError(t, err, "staying pack is no switch")
}

func TestPackIncrementalAndRestoreAfterPurge(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdatePack)
	r := newPackRepo(t)
	// incompressible, so the package of a small change stays well below the full one,
	// whose index alone takes over a kilobyte
	big := fmt.Sprintf("%x", randomBytes(t, 32<<10))
	_, n1, s1 := r.commit("2026-11-01T10:00:00+08:00", map[string]string{"big.txt": big, "a.txt": "1"})
	c2, n2, s2 := r.commit("2026-11-01T11:00:00+08:00", map[string]string{"a.txt": "2"})
	_, n3, s3 := r.commit("2026-11-01T12:00:00+08:00", map[string]string{"a.txt": "3"})
	_, n4, s4 := r.commit("2026-11-01T13:00:00+08:00", map[string]string{"a.txt": "4"})
	for _, u := range []struct {
		name     string
		snapshot []byte
	}{{n1, s1}, {n2, s2}} {
		status, err := uploadPack(t, l, u.name, u.snapshot)
		require.NoError(t, err)
		require.Equal(t, misc.StatusCompleted, status)
	}
	v1, v2 := versionID(t, client, n1), versionID(t, client, n2)

	// the incremental package from n1 to n2
	require.NoError(t, l.GenerateIncrementalPackage(ctx, "res", v2, v1, "", ""))
	patch, err := l.storageLogic.GetIncrementalUpdateStorage(ctx, v2, v1, "", "")
	require.NoError(t, err)
	meta, err := gitpack.ReadMeta(patch.PackagePath)
	require.NoError(t, err)
	full, err := l.storageLogic.GetFullUpdateStorage(ctx, v2, "", "")
	require.NoError(t, err)
	require.Equal(t, c2, meta.Commit)
	require.Less(t, patch.FileSize, full.FileSize)

	// both packages of a pack resource are served as pack
	latest, err := l.GetMultiLatestVersionInfo("res", "", "", "stable")
	require.NoError(t, err)
	for _, current := range []string{"", n1} {
		info, err := l.GetUpdateInfo(ctx, model.UpdateRequestParam{ResourceId: "res", CurrentVersionName: current, TargetVersionInfo: latest})
		require.NoError(t, err)
		require.Equal(t, types.UpdatePack.String(), info.UpdateType)
		require.Equal(t, current != "", strings.Contains(info.RelPath, "/patch/"))
	}

	// n1 and n2 fall out of the newest two and are purged, the package to n2 with them
	for _, u := range []struct {
		name     string
		snapshot []byte
	}{{n3, s3}, {n4, s4}} {
		status, err := uploadPack(t, l, u.name, u.snapshot)
		require.NoError(t, err)
		require.Equal(t, misc.StatusCompleted, status)
	}
	require.Empty(t, l.storageLogic.doPurgeResource(ctx, "res"))
	purged, err := l.storageLogic.GetFullUpdateStorage(ctx, v2, "", "")
	require.NoError(t, err)
	require.Empty(t, purged.PackagePath)
	require.NoFileExists(t, full.PackagePath)

	tag := misc.PatchGenerateTagKey("res", v2, v1, "", "")
	require.NoError(t, l.rdb.Set(ctx, tag, 1, time.Hour).Err())

	// the branch moves back to n2: its package comes back, the purged package to it goes
	status, err := uploadPack(t, l, n2, s2)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)
	require.Equal(t, n2, latestName(t, l))
	restored, err := l.storageLogic.GetFullUpdateStorage(ctx, v2, "", "")
	require.NoError(t, err)
	require.FileExists(t, restored.PackagePath)
	// named after its content: other bytes never take the URL a CDN may hold the old ones under
	require.Contains(t, filepath.Base(restored.PackagePath), restored.PackageHashSha256[:16])
	_, err = l.storageLogic.GetIncrementalUpdateStorage(ctx, v2, v1, "", "")
	require.True(t, ent.IsNotFound(err))
	exists, err := l.rdb.Exists(ctx, tag).Result()
	require.NoError(t, err)
	require.Zero(t, exists)

	// n1 lost its package, but its object IDs are enough to build the package from it
	require.NoError(t, l.GenerateIncrementalPackage(ctx, "res", v2, v1, "", ""))
	rebuilt, err := l.storageLogic.GetIncrementalUpdateStorage(ctx, v2, v1, "", "")
	require.NoError(t, err)
	require.FileExists(t, rebuilt.PackagePath)
	require.Equal(t, strconv.Itoa(v1), strings.SplitN(filepath.Base(rebuilt.PackagePath), "-", 2)[0])
}

// Releasing a version publishes it again in a more stable channel, and moves it there; an
// upload to a less stable channel changes nothing, as that channel gets the version anyway.
func TestPackRepublishMovesToMoreStableChannel(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdatePack)
	r := newPackRepo(t)
	_, n1, s1 := r.commit("2026-11-01T10:00:00+08:00", map[string]string{"a.txt": "1"})
	_, n2, s2 := r.commit("2026-11-01T11:00:00+08:00", map[string]string{"a.txt": "2"})
	upload := func(name, channel string, snapshot []byte) {
		t.Helper()
		status, err := uploadPackTo(t, l, name, channel, snapshot)
		require.NoError(t, err)
		require.Equal(t, misc.StatusCompleted, status)
	}
	channelOf := func(name string) version.Channel {
		return client.Version.Query().Where(version.Name(name)).OnlyX(ctx).Channel
	}

	upload(n1, "stable", s1)
	upload(n2, "beta", s2)
	require.Equal(t, n1, latestNameIn(t, l, "stable"))
	require.Equal(t, n2, latestNameIn(t, l, "beta"))

	upload(n2, "stable", s2)
	require.Equal(t, version.ChannelStable, channelOf(n2))
	require.Equal(t, n2, latestNameIn(t, l, "stable"))

	// moving beta back to n1 must not move stable back with it
	upload(n1, "beta", s1)
	require.Equal(t, version.ChannelStable, channelOf(n1))
	require.Equal(t, n2, latestNameIn(t, l, "stable"))
	require.Equal(t, n2, latestNameIn(t, l, "beta"))
}

// failCommand fails a redis command on the keys with a prefix while enabled.
type failCommand struct {
	name, prefix string
	enabled      bool
}

func (h *failCommand) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *failCommand) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *failCommand) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.enabled && cmd.Name() == h.name {
			for _, arg := range cmd.Args()[1:] {
				if key, ok := arg.(string); ok && strings.HasPrefix(key, h.prefix) {
					return errors.New("injected " + h.name + " failure")
				}
			}
		}
		return next(ctx, cmd)
	}
}

// The purged packages to a version go on every republish, so that a republish which
// failed to drop them is completed by the next one.
func TestPackRepublishDropsPurgedPatchesLeftBehind(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdatePack)
	r := newPackRepo(t)
	_, n1, s1 := r.commit("2026-11-01T10:00:00+08:00", map[string]string{"a.txt": "1"})
	_, n2, s2 := r.commit("2026-11-01T11:00:00+08:00", map[string]string{"a.txt": "2"})
	for _, u := range []struct {
		name     string
		snapshot []byte
	}{{n1, s1}, {n2, s2}} {
		status, err := uploadPack(t, l, u.name, u.snapshot)
		require.NoError(t, err)
		require.Equal(t, misc.StatusCompleted, status)
	}
	v1, v2 := versionID(t, client, n1), versionID(t, client, n2)
	// n2 got its package back, but the purged package to it outlived a failed republish
	client.Storage.Create().SetVersionID(v2).SetOldVersionID(v1).
		SetUpdateType(storage.UpdateTypeIncremental).SaveX(ctx)
	tag := misc.PatchGenerateTagKey("res", v2, v1, "", "")
	require.NoError(t, l.rdb.Set(ctx, tag, 1, time.Hour).Err())

	hook := &failCommand{name: "del", prefix: misc.GenerateTagKey + ":", enabled: true}
	l.rdb.AddHook(hook)
	status, err := uploadPack(t, l, n2, s2)
	require.ErrorContains(t, err, "injected del failure")
	require.Equal(t, misc.StatusFailed, status)
	_, err = l.storageLogic.GetIncrementalUpdateStorage(ctx, v2, v1, "", "")
	require.NoError(t, err, "kept for the next republish to drop with its tag")

	// the failed upload freed the version, so the next one can do it
	hook.enabled = false
	status, err = uploadPack(t, l, n2, s2)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)
	_, err = l.storageLogic.GetIncrementalUpdateStorage(ctx, v2, v1, "", "")
	require.True(t, ent.IsNotFound(err))
	exists, err := l.rdb.Exists(ctx, tag).Result()
	require.NoError(t, err)
	require.Zero(t, exists)
}

// A queued upload whose platform an admin moved into or out of pack meanwhile is
// rejected: its package would be served as what it is not.
func TestQueuedUploadRejectedAfterPackSwitch(t *testing.T) {
	r := newPackRepo(t)
	_, name, snapshot := r.commit("2026-11-01T10:00:00+08:00", map[string]string{"a.txt": "1"})
	for _, c := range []struct {
		name     string
		resource types.Update
		file     []byte
		filename string
		switchTo types.Update
	}{
		{"out of pack", types.UpdatePack, snapshot, "res.pack", types.UpdateFull},
		{"into pack", types.UpdateFull, []byte("installer"), "app.apk", types.UpdatePack},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			l, client := newPackTestLogic(t, c.resource)
			param := model.CreateVersionParam{ResourceID: "res", Name: name, Channel: "stable", Filename: c.filename}
			token, err := l.CreatePreSignedUrl(ctx, param)
			require.NoError(t, err)
			source := filepath.Join(l.storageLogic.OSSDir, token.Key)
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
			require.NoError(t, os.WriteFile(source, c.file, 0o644))
			statusKey, err := l.ProcessCreateVersionCallback(ctx, model.CreateVersionCallBackParam{
				ResourceID: "res", Name: name, Channel: "stable", Key: token.Key,
			})
			require.NoError(t, err)

			_, err = l.resourceLogic.SetPlatformUpdateType(ctx, "res", "", "", c.switchTo)
			require.NoError(t, err, "nothing is stored yet")
			queued := pendingUploads(t, l)
			require.Len(t, queued, 1)
			err = doHandleCalculatePackageHash(l.logger, l)(ctx, asynq.NewTask(queued[0].Type, queued[0].Payload))
			require.ErrorIs(t, err, asynq.SkipRetry)
			status, err := l.GetProcessingStatus(ctx, statusKey)
			require.NoError(t, err)
			require.Equal(t, misc.StatusFailed, status)
			require.Zero(t, client.Storage.Query().CountX(ctx))
			// the version is free for an upload that suits the platform
			param.Filename = "res.pack"
			_, err = l.CreatePreSignedUrl(ctx, param)
			require.NoError(t, err)
		})
	}
}

// The latest version is the one whose upload was accepted last, not the one processed
// last: a task that runs late, or runs again, cannot move the branch back.
func TestPackLateTasksDoNotMoveTheBranchBack(t *testing.T) {
	ctx := context.Background()
	l, _ := newPackTestLogic(t, types.UpdatePack)
	r := newPackRepo(t)
	_, n1, s1 := r.commit("2026-11-01T10:00:00+08:00", map[string]string{"a.txt": "1"})
	_, n2, s2 := r.commit("2026-11-01T11:00:00+08:00", map[string]string{"a.txt": "2"})
	run := func(task *asynq.Task) {
		t.Helper()
		require.NoError(t, doHandleCalculatePackageHash(l.logger, l)(ctx, task))
	}

	// n1 is accepted first, but its task is held up until n2 is through
	_, late, err := acceptPack(t, l, n1, "stable", s1)
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	status, err := uploadPack(t, l, n2, s2)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)

	run(late)
	require.Equal(t, n2, latestName(t, l), "stored late, n1 stays behind n2")
	run(late)
	require.Equal(t, n2, latestName(t, l), "published again late, n1 stays behind n2")

	// an upload of n1 accepted now moves the branch back
	status, err = uploadPack(t, l, n1, s1)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)
	require.Equal(t, n1, latestName(t, l))
	// and the late task, run once more, does not undo that
	run(late)
	require.Equal(t, n1, latestName(t, l))
}
