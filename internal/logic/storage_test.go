package logic

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/repo"
)

// newTestRepo opens a fresh in-memory sqlite database with the ent schema applied.
func newTestRepo(t *testing.T) (*repo.Repo, *ent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	// a single connection keeps every query on the same in-memory database
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	require.NoError(t, client.Schema.Create(context.Background()))

	return repo.NewRepo(client, sqlx.NewDb(db, "sqlite")), client
}

func newPurgeTestLogic(t *testing.T) (*StorageLogic, *ent.Client) {
	t.Helper()
	r, client := newTestRepo(t)
	dir := t.TempDir()
	return &StorageLogic{
		logger:      zap.NewNop(),
		storageRepo: repo.NewStorage(r),
		rawQuery:    repo.NewRawQuery(r),
		RootDir:     filepath.Join(dir, "storage"),
		OSSDir:      filepath.Join(dir, "oss"),
	}, client
}

// A version purged on one platform must not clear or orphan its other platforms
// that are still among the newest packages there.
func TestDoPurgeResourceKeepsOtherPlatformsOfSameVersion(t *testing.T) {
	ctx := context.Background()
	l, client := newPurgeTestLogic(t)

	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	type pkg struct {
		dir     string
		storage *ent.Storage
	}
	put := func(ver *ent.Version, os, arch string, updateType storage.UpdateType, at time.Time) pkg {
		platform := l.getPlatformDirName(os, arch)
		dir := filepath.Join(res.ID, strconv.Itoa(ver.ID), platform)
		for _, root := range []string{l.RootDir, l.OSSDir} {
			require.NoError(t, osMkdirWithFile(filepath.Join(root, dir)))
		}
		s := client.Storage.Create().
			SetVersion(ver).
			SetUpdateType(updateType).
			SetOs(os).
			SetArch(arch).
			SetPackagePath(filepath.Join(dir, "resource.zip")).
			SetCreatedAt(at).
			SaveX(ctx)
		return pkg{dir: dir, storage: s}
	}

	versions := make([]*ent.Version, 4)
	for i := range versions {
		versions[i] = client.Version.Create().
			SetResource(res).
			SetName("v" + strconv.Itoa(i+1)).
			SetNumber(uint64(i + 1)).
			SetCreatedAt(base.Add(time.Duration(i) * time.Hour)).
			SaveX(ctx)
	}
	at := func(i int) time.Time { return base.Add(time.Duration(i) * time.Hour) }

	// windows has v1..v4, so v1 and v2 fall out of the newest two;
	// linux only has v1, so it is still retained there.
	winV1 := put(versions[0], "windows", "amd64", storage.UpdateTypeFull, at(0))
	winV2 := put(versions[1], "windows", "amd64", storage.UpdateTypeFull, at(1))
	winV3 := put(versions[2], "windows", "amd64", storage.UpdateTypeFull, at(2))
	winV4 := put(versions[3], "windows", "amd64", storage.UpdateTypeFull, at(3))
	linV1 := put(versions[0], "linux", "amd64", storage.UpdateTypeFull, at(0))
	winV2Patch := client.Storage.Create().
		SetVersion(versions[1]).
		SetUpdateType(storage.UpdateTypeIncremental).
		SetOs("windows").
		SetArch("amd64").
		SetPackagePath(filepath.Join(winV2.dir, "patch", "p.zip")).
		SetCreatedAt(at(1)).
		SaveX(ctx)

	require.Empty(t, l.doPurgeResource(ctx, res.ID))

	packagePath := func(s *ent.Storage) string {
		return client.Storage.GetX(ctx, s.ID).PackagePath
	}

	// purged windows packages: files gone, package_path cleared (incremental included)
	for _, p := range []pkg{winV1, winV2} {
		require.NoDirExists(t, filepath.Join(l.RootDir, p.dir))
		require.NoDirExists(t, filepath.Join(l.OSSDir, p.dir))
		require.Empty(t, packagePath(p.storage))
	}
	require.Empty(t, packagePath(winV2Patch))

	// still-retained packages keep both files and package_path,
	// including linux v1 which shares its version with purged windows v1
	for _, p := range []pkg{winV3, winV4, linV1} {
		require.DirExists(t, filepath.Join(l.RootDir, p.dir))
		require.DirExists(t, filepath.Join(l.OSSDir, p.dir))
		require.NotEmpty(t, packagePath(p.storage))
	}

	// the local version dir is removed only when no platform is left in it:
	// v2 held only windows, v1 still holds linux
	require.NoDirExists(t, filepath.Join(l.RootDir, res.ID, strconv.Itoa(versions[1].ID)))
	require.DirExists(t, filepath.Join(l.RootDir, res.ID, strconv.Itoa(versions[0].ID)))
}

func TestRemoveDirIfEmpty(t *testing.T) {
	dir := t.TempDir()

	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.Mkdir(empty, 0o755))
	removeDirIfEmpty(empty)
	require.NoDirExists(t, empty)

	nonEmpty := filepath.Join(dir, "non-empty")
	require.NoError(t, osMkdirWithFile(nonEmpty))
	removeDirIfEmpty(nonEmpty)
	require.DirExists(t, nonEmpty)

	// missing dir is a no-op
	removeDirIfEmpty(filepath.Join(dir, "missing"))
}

func osMkdirWithFile(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "resource.zip"), []byte("x"), 0o644)
}
