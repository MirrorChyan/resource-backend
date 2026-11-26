package logic

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/storagefilehash"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/stretchr/testify/require"
)

func TestCorruptFileHashesReturnErrorWithoutSkipOrCachePoison(t *testing.T) {
	ctx := context.Background()
	l, client := newPatchTestLogic(t)
	l.cacheGroup.FullUpdateStorageCache = cache.NewWeightedCache[string, *model.FullStorage](-1, 64<<20, 1024, func(s *model.FullStorage) int64 {
		if s == nil {
			return 512
		}
		return int64(512 + len(s.HashData))
	})

	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	v1 := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)
	v2 := client.Version.Create().SetResource(res).SetName("v2").SetNumber(2).SaveX(ctx)

	s1, err := l.storageLogic.CreateFullUpdateStorage(ctx, v1.ID, "windows", "amd64", "/p/v1.zip", types.Zip, "h1", 100, map[string]string{"a.txt": "1"})
	require.NoError(t, err)
	s2, err := l.storageLogic.CreateFullUpdateStorage(ctx, v2.ID, "windows", "amd64", "/p/v2.zip", types.Zip, "h2", 100, map[string]string{"a.txt": "2"})
	require.NoError(t, err)

	// Corrupt v2's storage_file_hashes row.
	client.StorageFileHash.Update().
		Where(storagefilehash.StorageID(s2.ID)).
		SetData([]byte("corrupted-zstd")).
		ExecX(ctx)

	err = l.GenerateIncrementalPackage(ctx, res.ID, v2.ID, v1.ID, "windows", "amd64")
	require.Error(t, err)

	// Must NOT record patch:skip.
	skipped, err := l.rdb.Exists(ctx, misc.PatchSkipKey(v2.ID, v1.ID, "windows", "amd64")).Result()
	require.NoError(t, err)
	require.Zero(t, skipped)

	// Must NOT poison FullUpdateStorageCache with the corrupted row.
	cacheKey := l.cacheGroup.GetCacheKey(strconv.Itoa(v2.ID), "windows", "amd64")
	_, cached := l.cacheGroup.FullUpdateStorageCache.Get(cacheKey)
	require.False(t, cached)

	// Once repaired in DB, reading v2 succeeds immediately without manual cache eviction.
	goodHashes := map[string]string{"a.txt": "2"}
	enc, err := filehash.EncodeHashes(goodHashes)
	require.NoError(t, err)
	client.StorageFileHash.Update().
		Where(storagefilehash.StorageID(s2.ID)).
		SetFormat(enc.Format).
		SetData(enc.Data).
		SetRawSize(enc.RawSize).
		SetEntries(enc.Entries).
		ExecX(ctx)

	got, err := l.getFullUpdateStorageByCache(ctx, v2.ID, "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, goodHashes, got.fileHashes)
	require.Equal(t, s2.ID, got.storage.ID)
	_ = s1
}

func TestFullUpdateStorageCacheStoresCompressedFallbackAndServesHits(t *testing.T) {
	ctx := context.Background()
	l, client := newPatchTestLogic(t)
	l.cacheGroup.FullUpdateStorageCache = cache.NewWeightedCache[string, *model.FullStorage](-1, 64<<20, 1024, func(s *model.FullStorage) int64 {
		if s == nil {
			return 512
		}
		return int64(512 + len(s.HashData))
	})

	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	v1 := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)

	// Create a legacy row directly via ent (only in storages.file_hashes, absent from storage_file_hashes).
	legacyHashes := make(map[string]string, 100)
	for i := range 100 {
		legacyHashes["assets/dir/file_"+strconv.Itoa(i)+".png"] = strings.Repeat("a", 64)
	}
	st := client.Storage.Create().
		SetVersion(v1).
		SetUpdateType(storage.UpdateTypeFull).
		SetOs("windows").
		SetArch("amd64").
		SetPackagePath("/p/v1.zip").
		SetFileHashes(legacyHashes).
		SaveX(ctx)

	// First read falls back to legacy column and caches compressed bytes.
	got1, err := l.getFullUpdateStorageByCache(ctx, v1.ID, "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, legacyHashes, got1.fileHashes)

	cacheKey := l.cacheGroup.GetCacheKey(strconv.Itoa(v1.ID), "windows", "amd64")
	entry, ok := l.cacheGroup.FullUpdateStorageCache.Get(cacheKey)
	require.True(t, ok)
	require.Equal(t, filehash.FormatZstdJSON, entry.HashFormat)
	rawJSON, err := filehash.MarshalHashes(legacyHashes)
	require.NoError(t, err)
	require.Less(t, len(entry.HashData), len(rawJSON))

	// Corrupt legacy column in DB; second call must still succeed from compressed cache hit.
	client.Storage.UpdateOneID(st.ID).ClearFileHashes().ExecX(ctx)
	got2, err := l.getFullUpdateStorageByCache(ctx, v1.ID, "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, legacyHashes, got2.fileHashes)
}

func TestDoProcessPackStorageDoesNotReadNewFileHashesTable(t *testing.T) {
	ctx := context.Background()
	l, client := newPackTestLogic(t, types.UpdatePack)
	r := newPackRepo(t)
	_, n1, s1 := r.commit("2026-10-31T01:22:49+08:00", map[string]string{"a.txt": "1"})

	status, err := uploadPack(t, l, n1, s1)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)

	v1 := versionID(t, client, n1)
	st, err := l.storageLogic.GetFullUpdateStorage(ctx, v1, "", "")
	require.NoError(t, err)

	// Prime the cache, then corrupt storage_file_hashes.
	_, err = l.getFullUpdateStorageByCache(ctx, v1, "", "")
	require.NoError(t, err)
	client.StorageFileHash.Update().
		Where(storagefilehash.StorageID(st.ID)).
		SetData([]byte("corrupted")).
		ExecX(ctx)

	// Republishing only uses GetFullUpdateStorage (never touches storage_file_hashes)
	// and invalidates FullUpdateStorageCache.
	status, err = uploadPack(t, l, n1, s1)
	require.NoError(t, err)
	require.Equal(t, misc.StatusCompleted, status)

	cacheKey := l.cacheGroup.GetCacheKey(strconv.Itoa(v1), "", "")
	_, stillCached := l.cacheGroup.FullUpdateStorageCache.Get(cacheKey)
	require.False(t, stillCached)
}

func TestPatchSkippedWhenHashesMissingInBothTables(t *testing.T) {
	ctx := context.Background()
	l, client := newPatchTestLogic(t)
	l.cacheGroup.FullUpdateStorageCache = cache.NewCache[string, *model.FullStorage](-1, 1024)

	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	v1 := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)
	v2 := client.Version.Create().SetResource(res).SetName("v2").SetNumber(2).SaveX(ctx)

	p2 := filepath.Join(t.TempDir(), "v2.zip")
	require.NoError(t, os.WriteFile(p2, []byte("zip"), 0o644))

	_, err := l.storageLogic.CreateFullUpdateStorage(ctx, v1.ID, "windows", "amd64", "/p/v1.zip", types.Zip, "h1", 100, nil)
	require.NoError(t, err)
	_, err = l.storageLogic.CreateFullUpdateStorage(ctx, v2.ID, "windows", "amd64", p2, types.Zip, "h2", 100, map[string]string{"a.txt": "2"})
	require.NoError(t, err)

	require.NoError(t, l.GenerateIncrementalPackage(ctx, res.ID, v2.ID, v1.ID, "windows", "amd64"))
	skipped, err := l.rdb.Exists(ctx, misc.PatchSkipKey(v2.ID, v1.ID, "windows", "amd64")).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), skipped)
}
