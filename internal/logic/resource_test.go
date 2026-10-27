package logic

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/MirrorChyan/resource-backend/internal/repo"
)

// newTestCacheGroup returns the caches the logic under test reads.
func newTestCacheGroup() *cache.MultiCacheGroup {
	return &cache.MultiCacheGroup{
		VersionNameIdCache:         cache.NewCache[string, int](-1),
		IncrementalUpdateInfoCache: cache.NewCache[string, *model.IncrementalUpdateInfo](time.Hour),
		ResourceInfoCache:          cache.NewCache[string, *ent.Resource](-1),
		PlatformUpdateTypeCache:    cache.NewCache[string, map[model.Platform]types.Update](-1),
	}
}

func newTestResourceLogic(r *repo.Repo, rdb *redis.Client, cg *cache.MultiCacheGroup) *ResourceLogic {
	return NewResourceLogic(zap.NewNop(),
		repo.NewResource(r), repo.NewPlatformConfig(r), repo.NewStorage(r),
		rdb, cg,
	)
}

func newResourceTestLogic(t *testing.T) (*ResourceLogic, *ent.Client, *redis.Client) {
	t.Helper()
	r, client := newTestRepo(t)
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return newTestResourceLogic(r, rdb, newTestCacheGroup()), client, rdb
}

func TestResolveUpdateTypeFallsBackToResource(t *testing.T) {
	ctx := context.Background()
	l, client, _ := newResourceTestLogic(t)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").
		SetUpdateType(types.UpdateIncremental.String()).SaveX(ctx)

	_, err := l.SetPlatformUpdateType(ctx, "res", "android", "arm64", types.UpdateFull)
	require.NoError(t, err)
	_, err = l.SetPlatformUpdateType(ctx, "res", "", "", types.UpdateFull)
	require.NoError(t, err)

	for _, c := range []struct {
		os, arch string
		want     types.Update
	}{
		{"android", "arm64", types.UpdateFull},
		{"", "", types.UpdateFull},
		{"windows", "amd64", types.UpdateIncremental},
		// "" names the platform-independent package, it matches no other platform
		{"android", "", types.UpdateIncremental},
		{"", "arm64", types.UpdateIncremental},
	} {
		got, err := l.ResolveUpdateType(ctx, "res", c.os, c.arch)
		require.NoError(t, err)
		require.Equal(t, c.want, got, "%s/%s", c.os, c.arch)
	}
}

func TestResolveUpdateTypeOfUnknownResource(t *testing.T) {
	l, _, _ := newResourceTestLogic(t)
	_, err := l.ResolveUpdateType(context.Background(), "missing", "", "")
	require.True(t, ent.IsNotFound(err))
}

// A change shows on this instance at once and asks the other instances to drop the
// platform update types of the resource.
func TestSetPlatformUpdateTypeEvictsCache(t *testing.T) {
	ctx := context.Background()
	l, client, rdb := newResourceTestLogic(t)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)

	sub := rdb.Subscribe(ctx, "evict:platform-update-type")
	t.Cleanup(func() { _ = sub.Close() })
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	got, err := l.ResolveUpdateType(ctx, "res", "linux", "arm64")
	require.NoError(t, err)
	require.Equal(t, types.UpdateIncremental, got)

	_, err = l.SetPlatformUpdateType(ctx, "res", "linux", "arm64", types.UpdateFull)
	require.NoError(t, err)
	got, err = l.ResolveUpdateType(ctx, "res", "linux", "arm64")
	require.NoError(t, err)
	require.Equal(t, types.UpdateFull, got)
	select {
	case msg := <-sub.Channel():
		require.Equal(t, "res", msg.Payload)
	case <-time.After(5 * time.Second):
		t.Fatal("no evict message published")
	}

	_, err = l.DeletePlatformUpdateType(ctx, "res", "linux", "arm64")
	require.NoError(t, err)
	got, err = l.ResolveUpdateType(ctx, "res", "linux", "arm64")
	require.NoError(t, err)
	require.Equal(t, types.UpdateIncremental, got)
}

func TestPlatformUpdateTypeChanges(t *testing.T) {
	ctx := context.Background()
	l, client, _ := newResourceTestLogic(t)
	client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)

	inherit := model.PlatformUpdateTypeState{UpdateType: "incremental", Source: model.PlatformSourceInherit}
	ownFull := model.PlatformUpdateTypeState{UpdateType: "full", Source: model.PlatformSourceOverride}
	ownIncremental := model.PlatformUpdateTypeState{UpdateType: "incremental", Source: model.PlatformSourceOverride}

	change, err := l.SetPlatformUpdateType(ctx, "res", "android", "arm64", types.UpdateFull)
	require.NoError(t, err)
	require.Equal(t, inherit, change.Before)
	require.Equal(t, ownFull, change.After)

	// setting it again updates the one row of the platform
	change, err = l.SetPlatformUpdateType(ctx, "res", "android", "arm64", types.UpdateIncremental)
	require.NoError(t, err)
	require.Equal(t, ownFull, change.Before)
	require.Equal(t, ownIncremental, change.After)
	require.Equal(t, 1, client.ResourcePlatformConfig.Query().CountX(ctx))

	change, err = l.DeletePlatformUpdateType(ctx, "res", "android", "arm64")
	require.NoError(t, err)
	require.Equal(t, ownIncremental, change.Before)
	require.Equal(t, inherit, change.After)
	require.Zero(t, client.ResourcePlatformConfig.Query().CountX(ctx))

	// returning a platform that has nothing of its own leaves it as is
	change, err = l.DeletePlatformUpdateType(ctx, "res", "android", "arm64")
	require.NoError(t, err)
	require.Equal(t, inherit, change.Before)
	require.Equal(t, inherit, change.After)
}

func TestPlatformUpdateTypeOfUnknownResource(t *testing.T) {
	ctx := context.Background()
	l, _, _ := newResourceTestLogic(t)

	_, err := l.SetPlatformUpdateType(ctx, "missing", "", "", types.UpdateFull)
	require.ErrorIs(t, err, errs.ErrResourceNotFound)
	_, err = l.DeletePlatformUpdateType(ctx, "missing", "", "")
	require.ErrorIs(t, err, errs.ErrResourceNotFound)
	_, err = l.ListPlatformUpdateTypes(ctx, "missing")
	require.ErrorIs(t, err, errs.ErrResourceNotFound)
}

// The list holds the platforms with packages and the ones with their own update type, once each.
func TestListPlatformUpdateTypes(t *testing.T) {
	ctx := context.Background()
	l, client, _ := newResourceTestLogic(t)
	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	v1 := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)
	v2 := client.Version.Create().SetResource(res).SetName("v2").SetNumber(2).SaveX(ctx)
	for _, s := range []struct {
		ver      *ent.Version
		os, arch string
	}{
		{v1, "windows", "amd64"},
		{v2, "windows", "amd64"},
		{v2, "android", "arm64"},
	} {
		client.Storage.Create().SetVersion(s.ver).SetOs(s.os).SetArch(s.arch).
			SetUpdateType(storage.UpdateTypeFull).SaveX(ctx)
	}
	// a patch is no platform of its own
	client.Storage.Create().SetVersion(v2).SetOldVersion(v1).SetOs("darwin").SetArch("arm64").
		SetUpdateType(storage.UpdateTypeIncremental).SaveX(ctx)

	_, err := l.SetPlatformUpdateType(ctx, "res", "android", "arm64", types.UpdateFull)
	require.NoError(t, err)
	_, err = l.SetPlatformUpdateType(ctx, "res", "linux", "arm64", types.UpdateFull)
	require.NoError(t, err)

	data, err := l.ListPlatformUpdateTypes(ctx, "res")
	require.NoError(t, err)
	require.Equal(t, "incremental", data.DefaultUpdateType)
	require.Len(t, data.Platforms, 3)

	android, linux, windows := data.Platforms[0], data.Platforms[1], data.Platforms[2]
	require.Equal(t, model.Platform{OS: "android", Arch: "arm64"}, model.Platform{OS: android.OS, Arch: android.Arch})
	require.Equal(t, "full", android.UpdateType)
	require.Equal(t, model.PlatformSourceOverride, android.Source)
	require.True(t, android.HasStorage)
	require.NotNil(t, android.UpdatedAt)

	require.Equal(t, model.Platform{OS: "linux", Arch: "arm64"}, model.Platform{OS: linux.OS, Arch: linux.Arch})
	require.Equal(t, model.PlatformSourceOverride, linux.Source)
	require.False(t, linux.HasStorage)

	require.Equal(t, model.Platform{OS: "windows", Arch: "amd64"}, model.Platform{OS: windows.OS, Arch: windows.Arch})
	require.Equal(t, "incremental", windows.UpdateType)
	require.Equal(t, model.PlatformSourceInherit, windows.Source)
	require.True(t, windows.HasStorage)
	require.Nil(t, windows.UpdatedAt)

	counts, err := l.CountPlatformOverrides(ctx, []string{"res", "other"})
	require.NoError(t, err)
	require.Equal(t, map[string]int{"res": 2}, counts)
}
