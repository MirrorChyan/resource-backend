package repo

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/storagefilehash"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type queryRecorderDriver struct {
	dialect.Driver
	mu      sync.Mutex
	queries []string
}

func (d *queryRecorderDriver) Query(ctx context.Context, query string, args, v any) error {
	d.mu.Lock()
	d.queries = append(d.queries, query)
	d.mu.Unlock()
	return d.Driver.Query(ctx, query, args, v)
}

func (d *queryRecorderDriver) reset() {
	d.mu.Lock()
	d.queries = nil
	d.mu.Unlock()
}

func (d *queryRecorderDriver) recorded() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.queries))
	copy(out, d.queries)
	return out
}

func newRepoTestEnv(t *testing.T) (*Storage, *ent.Client, *sqlx.DB, *queryRecorderDriver) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	rec := &queryRecorderDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
	client := ent.NewClient(ent.Driver(rec))
	require.NoError(t, client.Schema.Create(context.Background()))

	dx := sqlx.NewDb(db, "sqlite")
	return NewStorage(NewRepo(client, dx)), client, dx, rec
}

func seedVersions(t *testing.T, client *ent.Client) (*ent.Version, *ent.Version) {
	t.Helper()
	ctx := context.Background()
	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	v1 := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)
	v2 := client.Version.Create().SetResource(res).SetName("v2").SetNumber(2).SaveX(ctx)
	return v1, v2
}

func TestDualWriteAndEmptyHashesSkipNewTable(t *testing.T) {
	ctx := context.Background()
	stRepo, client, dx, _ := newRepoTestEnv(t)
	v1, v2 := seedVersions(t, client)

	hashes := map[string]string{
		"bin/app.exe": strings.Repeat("a", 64),
		"README.md":   strings.Repeat("b", 64),
	}
	full, err := stRepo.CreateFullUpdateStorage(ctx, v1.ID, "windows", "amd64", "/p/1.zip", "h1", types.Zip, 100, hashes)
	require.NoError(t, err)

	// New table has compressed row.
	row, err := client.StorageFileHash.Query().Where(storagefilehash.StorageID(full.ID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, filehash.FormatZstdJSON, row.Format)
	require.Equal(t, len(hashes), row.Entries)
	require.Greater(t, row.RawSize, 0)
	decoded, err := filehash.DecodeHashes(row.Format, row.Data)
	require.NoError(t, err)
	require.Equal(t, hashes, decoded)

	// Legacy column was also written in Step 1.
	var legacyJSON sql.NullString
	require.NoError(t, dx.QueryRowContext(ctx, "SELECT file_hashes FROM storages WHERE id = ?", full.ID).Scan(&legacyJSON))
	require.True(t, legacyJSON.Valid)
	legacyMap, _, err := filehash.ParseRawHashesJSON([]byte(legacyJSON.String), legacyJSON.Valid)
	require.NoError(t, err)
	require.Equal(t, hashes, legacyMap)

	// Pack storage dual-writes as well.
	packObjs := map[string]string{".": strings.Repeat("1", 40), "a.txt": strings.Repeat("2", 40)}
	packSt, err := stRepo.CreatePackStorage(ctx, v2.ID, "", "", "/p/pack.zip", "h2", 200, packObjs, strings.Repeat("c", 40), time.Now())
	require.NoError(t, err)
	packRow, err := client.StorageFileHash.Query().Where(storagefilehash.StorageID(packSt.ID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, packRow.Entries)

	// Nil and empty map do NOT create a row in storage_file_hashes.
	nilSt, err := stRepo.CreateFullUpdateStorage(ctx, v1.ID, "android", "arm64", "/p/app.apk", "h3", "", 300, nil)
	require.NoError(t, err)
	emptySt, err := stRepo.CreateFullUpdateStorage(ctx, v2.ID, "linux", "amd64", "/p/app.bin", "h4", "", 400, map[string]string{})
	require.NoError(t, err)

	exists, err := client.StorageFileHash.Query().Where(storagefilehash.StorageIDIn(nilSt.ID, emptySt.ID)).Exist(ctx)
	require.NoError(t, err)
	require.False(t, exists)

	// Deleting a storage cascades to storage_file_hashes.
	require.NoError(t, stRepo.DeleteStorages(ctx, full.ID))
	stillThere, err := client.StorageFileHash.Query().Where(storagefilehash.StorageID(full.ID)).Exist(ctx)
	require.NoError(t, err)
	require.False(t, stillThere)
}

func TestGetFileHashesPriorityFallbackEmptyAndCorrupt(t *testing.T) {
	ctx := context.Background()
	stRepo, client, dx, _ := newRepoTestEnv(t)
	v1, v2 := seedVersions(t, client)

	readCounter := func(label string) float64 {
		return testutil.ToFloat64(FileHashesReadsCounter().WithLabelValues(label))
	}

	// 1. New table takes priority over legacy column.
	st1 := client.Storage.Create().
		SetVersion(v1).
		SetUpdateType(storage.UpdateTypeFull).
		SetOs("windows").
		SetArch("amd64").
		SetFileHashes(map[string]string{"old.txt": "old"}).
		SaveX(ctx)
	newMap := map[string]string{"new.txt": "new"}
	enc, err := filehash.EncodeHashes(newMap)
	require.NoError(t, err)
	client.StorageFileHash.Create().
		SetStorageID(st1.ID).
		SetFormat(enc.Format).
		SetData(enc.Data).
		SetRawSize(enc.RawSize).
		SetEntries(enc.Entries).
		SaveX(ctx)

	beforeNew := readCounter(FileHashesReadNew)
	got, err := stRepo.GetFileHashes(ctx, st1.ID)
	require.NoError(t, err)
	require.Equal(t, newMap, got)
	require.Equal(t, beforeNew+1, readCounter(FileHashesReadNew))

	// 2. Fallback to legacy column when new table has no row; returns compressed format=1 for cache.
	st2 := client.Storage.Create().
		SetVersion(v2).
		SetUpdateType(storage.UpdateTypeFull).
		SetOs("windows").
		SetArch("amd64").
		SetFileHashes(map[string]string{"legacy.txt": "abc"}).
		SaveX(ctx)

	beforeFallback := readCounter(FileHashesReadFallbackOld)
	full, hashes, err := stRepo.GetFullUpdateStorageWithHashes(ctx, v2.ID, "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, st2.ID, full.Storage.ID)
	require.Equal(t, map[string]string{"legacy.txt": "abc"}, hashes)
	require.Equal(t, filehash.FormatZstdJSON, full.HashFormat)
	require.NotEmpty(t, full.HashData)
	require.Empty(t, full.Storage.FileHashes)
	require.Equal(t, beforeFallback+1, readCounter(FileHashesReadFallbackOld))

	// 3. Empty legacy values: SQL NULL, JSON null, JSON {} all return (nil, nil) and increment miss.
	for _, tc := range []struct {
		os  string
		sql string
	}{
		{"snull", "NULL"},
		{"jnull", "'null'"},
		{"empty", "'{}'"},
	} {
		st := client.Storage.Create().
			SetVersion(v1).
			SetUpdateType(storage.UpdateTypeFull).
			SetOs(tc.os).
			SetArch("amd64").
			SaveX(ctx)
		_, err := dx.ExecContext(ctx, "UPDATE storages SET file_hashes = "+tc.sql+" WHERE id = ?", st.ID)
		require.NoError(t, err)

		beforeMiss := readCounter(FileHashesReadMiss)
		got, err := stRepo.GetFileHashes(ctx, st.ID)
		require.NoError(t, err)
		require.Nil(t, got)
		require.Equal(t, beforeMiss+1, readCounter(FileHashesReadMiss))
	}

	// 4. Corrupted new table rows return error and increment error counter.
	for _, tc := range []struct {
		os     string
		format uint8
		data   []byte
	}{
		{"badfmt", 99, []byte("abc")},
		{"badzstd", filehash.FormatZstdJSON, []byte("not-zstd")},
		{"badjson", filehash.FormatZstdJSON, filehash.CompressRawJSON([]byte("{broken"), false)},
	} {
		st := client.Storage.Create().
			SetVersion(v2).
			SetUpdateType(storage.UpdateTypeFull).
			SetOs(tc.os).
			SetArch("amd64").
			SaveX(ctx)
		client.StorageFileHash.Create().
			SetStorageID(st.ID).
			SetFormat(tc.format).
			SetData(tc.data).
			SetRawSize(10).
			SetEntries(1).
			SaveX(ctx)

		beforeErr := readCounter(FileHashesReadError)
		_, err := stRepo.GetFileHashes(ctx, st.ID)
		require.Error(t, err, tc.os)
		require.Equal(t, beforeErr+1, readCounter(FileHashesReadError))
	}
}

func TestQueriesExcludeFileHashesAndKeepOldVersionEdge(t *testing.T) {
	ctx := context.Background()
	stRepo, client, _, rec := newRepoTestEnv(t)
	v1, v2 := seedVersions(t, client)

	// Verify dynamic column list covers every Storage column except file_hashes.
	require.NotContains(t, storageColumnsWithoutFileHashes, storage.FieldFileHashes)
	require.Len(t, storageColumnsWithoutFileHashes, len(storage.Columns)-1)

	full, err := stRepo.CreateFullUpdateStorage(ctx, v2.ID, "windows", "amd64", "/p/full.zip", "hf", types.Zip, 100, map[string]string{"a": "b"})
	require.NoError(t, err)

	// One retained incremental package and one purged incremental package (package_path IS NULL).
	incRetained := client.Storage.Create().
		SetVersion(v2).
		SetOldVersion(v1).
		SetUpdateType(storage.UpdateTypeIncremental).
		SetOs("windows").
		SetArch("amd64").
		SetPackagePath("/p/patch.zip").
		SaveX(ctx)
	incPurged := client.Storage.Create().
		SetVersion(v2).
		SetOldVersion(v1).
		SetUpdateType(storage.UpdateTypeIncremental).
		SetOs("linux").
		SetArch("amd64").
		SaveX(ctx)

	assertNoFileHashesInSQL := func(name string) {
		t.Helper()
		queries := rec.recorded()
		require.NotEmpty(t, queries, name)
		for _, q := range queries {
			require.NotContains(t, strings.ToLower(q), "file_hashes", "%s queried file_hashes: %s", name, q)
		}
	}

	// 1. GetFullUpdateStorage
	rec.reset()
	gotFull, err := stRepo.GetFullUpdateStorage(ctx, v2.ID, "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, full.ID, gotFull.ID)
	require.Equal(t, "/p/full.zip", gotFull.PackagePath)
	require.Empty(t, gotFull.FileHashes)
	assertNoFileHashesInSQL("GetFullUpdateStorage")

	// 2. GetIncrementalUpdateStorage
	rec.reset()
	gotInc, err := stRepo.GetIncrementalUpdateStorage(ctx, v2.ID, v1.ID, "windows", "amd64")
	require.NoError(t, err)
	require.Equal(t, incRetained.ID, gotInc.ID)
	assertNoFileHashesInSQL("GetIncrementalUpdateStorage")

	// 3. ListPurgedIncrementalStorages (must still load Edges.OldVersion)
	rec.reset()
	purgedRows, err := stRepo.ListPurgedIncrementalStorages(ctx, v2.ID, "linux", "amd64")
	require.NoError(t, err)
	require.Len(t, purgedRows, 1)
	require.Equal(t, incPurged.ID, purgedRows[0].ID)
	require.NotNil(t, purgedRows[0].Edges.OldVersion)
	require.Equal(t, v1.ID, purgedRows[0].Edges.OldVersion.ID)
	assertNoFileHashesInSQL("ListPurgedIncrementalStorages")

	// 4. PurgeStorageInfo
	rec.reset()
	require.NoError(t, stRepo.PurgeStorageInfo(ctx, full.ID))
	assertNoFileHashesInSQL("PurgeStorageInfo")
	cleared, err := stRepo.GetFullUpdateStorage(ctx, v2.ID, "windows", "amd64")
	require.NoError(t, err)
	require.Empty(t, cleared.PackagePath)
}
