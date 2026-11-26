package migration

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v9"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

type migratorEnv struct {
	client *ent.Client
	dx     *sqlx.DB
	mr     *miniredis.Miniredis
	rdb    *redis.Client
	dl     *redsync.Redsync
	ver    *ent.Version
}

func newMigratorEnv(t *testing.T) *migratorEnv {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	require.NoError(t, client.Schema.Create(context.Background()))

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	dl := redsync.New(goredis.NewPool(rdb))

	ctx := context.Background()
	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	ver := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)

	return &migratorEnv{
		client: client,
		dx:     sqlx.NewDb(db, "sqlite"),
		mr:     mr,
		rdb:    rdb,
		dl:     dl,
		ver:    ver,
	}
}

func (e *migratorEnv) startMigrator(t *testing.T, batchSize int) *FileHashesMigrator {
	t.Helper()
	m := NewFileHashesMigrator(zap.NewNop(), e.dx, e.rdb, e.dl)
	m.BatchSize = batchSize
	m.SampleSize = 50
	m.LockExpiry = 2 * time.Second
	m.LockRenewInterval = 400 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = m.Start(ctx) }()
	select {
	case <-m.Ready():
	case <-time.After(3 * time.Second):
		t.Fatal("migrator did not become ready")
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = m.Stop(stopCtx)
		cancel()
	})
	return m
}

func (e *migratorEnv) insertLegacyFull(t *testing.T, osName string, rawSQL string) int {
	t.Helper()
	ctx := context.Background()
	st := e.client.Storage.Create().
		SetVersion(e.ver).
		SetUpdateType(storage.UpdateTypeFull).
		SetOs(osName).
		SetArch("amd64").
		SaveX(ctx)
	_, err := e.dx.ExecContext(ctx, "UPDATE storages SET file_hashes = "+rawSQL+" WHERE id = ?", st.ID)
	require.NoError(t, err)
	return st.ID
}

func (e *migratorEnv) insertIncremental(t *testing.T, osName string) int {
	t.Helper()
	ctx := context.Background()
	st := e.client.Storage.Create().
		SetVersion(e.ver).
		SetOldVersion(e.ver).
		SetUpdateType(storage.UpdateTypeIncremental).
		SetOs(osName).
		SetArch("amd64").
		SaveX(ctx)
	return st.ID
}

func (e *migratorEnv) publish(t *testing.T, cmd string) {
	t.Helper()
	require.NoError(t, e.rdb.Publish(context.Background(), Channel, cmd).Err())
}

func (e *migratorEnv) waitHashStatus(t *testing.T, key, want string) map[string]string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h, err := e.rdb.HGetAll(context.Background(), key).Result()
		if err == nil && h["status"] == want {
			return h
		}
		time.Sleep(20 * time.Millisecond)
	}
	h, _ := e.rdb.HGetAll(context.Background(), key).Result()
	t.Fatalf("timed out waiting for %s status=%s, got=%v", key, want, h)
	return nil
}

func TestPubSubBackfillVerifyStatsAndReset(t *testing.T) {
	env := newMigratorEnv(t)
	env.startMigrator(t, 3)
	ctx := context.Background()

	// Seed legacy rows: 4 non-empty (1 already in new table), 3 empties (SQL NULL, JSON null, {}), 2 incremental.
	idAlready := env.insertLegacyFull(t, "os1", `'{"a.txt":"111","b.txt":"222"}'`)
	enc, err := filehash.EncodeHashes(map[string]string{"a.txt": "111", "b.txt": "222"})
	require.NoError(t, err)
	env.client.StorageFileHash.Create().
		SetStorageID(idAlready).
		SetFormat(enc.Format).
		SetData(enc.Data).
		SetRawSize(enc.RawSize).
		SetEntries(enc.Entries).
		SaveX(ctx)

	idNonEmpty1 := env.insertLegacyFull(t, "os2", `'{"c.txt":"333"}'`)
	idNonEmpty2 := env.insertLegacyFull(t, "os3", `'{"d.txt":"444","e.txt":"555"}'`)
	idNonEmpty3 := env.insertLegacyFull(t, "os4", `'{"f.txt":"666"}'`)
	env.insertLegacyFull(t, "os5", "NULL")
	env.insertLegacyFull(t, "os6", "'null'")
	env.insertLegacyFull(t, "os7", "'{}'")
	env.insertIncremental(t, "inc1")
	env.insertIncremental(t, "inc2")

	// 1. Stats before backfill does not write new table or touch StateKey.
	env.publish(t, "stats 50")
	stats := env.waitHashStatus(t, StatsKey, StatusDone)
	require.Equal(t, "4", stats["non_empty"])
	require.Equal(t, "1", stats["sql_null"])
	require.Equal(t, "1", stats["json_null"])
	require.Equal(t, "1", stats["empty_object"])
	require.Equal(t, int64(0), env.rdb.Exists(ctx, StateKey).Val())
	require.Equal(t, 1, env.client.StorageFileHash.Query().CountX(ctx))

	// 2. Start backfill.
	env.publish(t, "start 10")
	state := env.waitHashStatus(t, StateKey, StatusDone)
	require.Equal(t, "3", state["written"])
	require.Equal(t, "1", state["skipped"])
	require.Equal(t, "4", state["non_empty"])
	require.Equal(t, "3", state["empty"])
	require.Equal(t, 4, env.client.StorageFileHash.Query().CountX(ctx))

	// Check decoded contents of backfilled rows.
	for _, id := range []int{idNonEmpty1, idNonEmpty2, idNonEmpty3} {
		var (
			format uint8
			data   []byte
		)
		require.NoError(t, env.dx.QueryRowContext(ctx, "SELECT format, data FROM storage_file_hashes WHERE storage_id = ?", id).Scan(&format, &data))
		got, err := filehash.DecodeHashes(format, data)
		require.NoError(t, err)
		require.NotEmpty(t, got)
	}

	// 3. A new full row created after max_id (simulating Step 1 dual-write) does not break verify's <= max_id count check.
	idPostBound := env.insertLegacyFull(t, "os_new", `'{"z.txt":"999"}'`)
	encPost, err := filehash.EncodeHashes(map[string]string{"z.txt": "999"})
	require.NoError(t, err)
	env.client.StorageFileHash.Create().
		SetStorageID(idPostBound).
		SetFormat(encPost.Format).
		SetData(encPost.Data).
		SetRawSize(encPost.RawSize).
		SetEntries(encPost.Entries).
		SaveX(ctx)

	env.publish(t, "verify 50")
	verify := env.waitHashStatus(t, VerifyKey, StatusDone)
	require.Equal(t, "true", verify["count_match"])
	require.Equal(t, "4", verify["new_table_count"])
	require.Equal(t, "4", verify["state_non_empty"])
	require.Equal(t, "4", verify["matched"])
	require.Equal(t, "3", verify["empty_ok"])
	require.Equal(t, "0", verify["missing"])
	require.Equal(t, "0", verify["mismatched"])
	require.Equal(t, "0", verify["errors"])

	// 4. Publishing start again when already done is a no-op (keeps max_id and counters).
	oldMaxID := state["max_id"]
	env.publish(t, "start")
	time.Sleep(150 * time.Millisecond)
	afterRepeat, err := env.rdb.HGetAll(ctx, StateKey).Result()
	require.NoError(t, err)
	require.Equal(t, StatusDone, afterRepeat["status"])
	require.Equal(t, oldMaxID, afterRepeat["max_id"])
	require.Equal(t, "3", afterRepeat["written"])

	// 5. Reset clears state; rerunning skips every already-backfilled row.
	env.publish(t, "reset")
	require.Eventually(t, func() bool {
		return env.rdb.Exists(ctx, StateKey).Val() == 0
	}, 2*time.Second, 20*time.Millisecond)

	env.publish(t, "start 10")
	rerun := env.waitHashStatus(t, StateKey, StatusDone)
	require.Equal(t, "0", rerun["written"])
	require.Equal(t, "5", rerun["skipped"])
	require.Equal(t, "5", rerun["non_empty"])
	require.Equal(t, "3", rerun["empty"])

	// 6. Verify detects corrupted/mismatched content.
	badEnc, err := filehash.EncodeHashes(map[string]string{"wrong.txt": "000"})
	require.NoError(t, err)
	_, err = env.dx.ExecContext(ctx, "UPDATE storage_file_hashes SET data = ? WHERE storage_id = ?", badEnc.Data, idNonEmpty1)
	require.NoError(t, err)
	require.NoError(t, env.rdb.Del(ctx, VerifyKey).Err())
	env.publish(t, "verify 50")
	verifyBad := env.waitHashStatus(t, VerifyKey, StatusDone)
	require.Equal(t, "1", verifyBad["mismatched"])
}

func TestResumePreservesMaxIDAndStaleRunningStatus(t *testing.T) {
	env := newMigratorEnv(t)
	env.startMigrator(t, 2)
	ctx := context.Background()

	id1 := env.insertLegacyFull(t, "os1", `'{"a":"1"}'`)
	id2 := env.insertLegacyFull(t, "os2", `'{"b":"2"}'`)
	id3 := env.insertLegacyFull(t, "os3", `'{"c":"3"}'`)
	id4 := env.insertLegacyFull(t, "os4", `'{"d":"4"}'`)
	// Post-max_id row (added after initial max_id=id3) must not be touched by resume.
	_ = id4

	// Pre-populate id1 in new table and simulate a hard-killed container leaving status=running
	// with no active lock, max_id=id3, cursor=id1, written=1.
	enc1, err := filehash.EncodeHashes(map[string]string{"a": "1"})
	require.NoError(t, err)
	env.client.StorageFileHash.Create().
		SetStorageID(id1).
		SetFormat(enc1.Format).
		SetData(enc1.Data).
		SetRawSize(enc1.RawSize).
		SetEntries(enc1.Entries).
		SaveX(ctx)

	require.NoError(t, env.rdb.HSet(ctx, StateKey,
		"status", StatusRunning,
		"max_id", id3,
		"cursor", id1,
		"written", 1,
		"skipped", 0,
		"non_empty", 1,
		"empty", 0,
		"rate_mb", "5",
	).Err())

	env.publish(t, "start")
	done := env.waitHashStatus(t, StateKey, StatusDone)
	require.Equal(t, strconv.Itoa(id3), done["max_id"])
	require.Equal(t, strconv.Itoa(id3), done["cursor"])
	require.Equal(t, "3", done["written"])
	require.Equal(t, "3", done["non_empty"])
	// Only id1, id2, id3 were backfilled; id4 > max_id was not touched.
	require.Equal(t, 3, env.client.StorageFileHash.Query().CountX(ctx))
	_ = id2
}

func TestLockContentionStopRateAndBounds(t *testing.T) {
	env := newMigratorEnv(t)
	// Start two migrators to simulate two live containers receiving the same Pub/Sub messages.
	env.startMigrator(t, 1)
	env.startMigrator(t, 1)
	ctx := context.Background()

	// Seed large payloads (~256 KiB each) so at 0.5 MB/s the backfill takes several seconds.
	bigVal := fmt.Sprintf(`{"blob":"%s"}`, strings.Repeat("x", 256<<10))
	for i := range 6 {
		env.insertLegacyFull(t, fmt.Sprintf("big_%d", i), "'"+bigVal+"'")
	}

	// Out-of-range start rate is ignored completely.
	env.publish(t, "start 20")
	env.publish(t, "start 0.1")
	env.publish(t, "start -1")
	time.Sleep(150 * time.Millisecond)
	require.Equal(t, int64(0), env.rdb.Exists(ctx, StateKey).Val())

	// Start at 0.5 MB/s; both instances receive it, only one acquires lock.
	env.publish(t, "start 0.5")
	env.waitHashStatus(t, StateKey, StatusRunning)

	// While running: stats, verify, reset are all rejected because the shared lock is held.
	env.publish(t, "stats 10")
	env.publish(t, "verify 10")
	env.publish(t, "reset")
	time.Sleep(150 * time.Millisecond)
	require.Equal(t, int64(0), env.rdb.Exists(ctx, StatsKey).Val())
	require.Equal(t, int64(0), env.rdb.Exists(ctx, VerifyKey).Val())
	require.Equal(t, int64(1), env.rdb.Exists(ctx, StateKey).Val())

	// Out-of-range rate changes are ignored; valid rate change takes effect.
	env.publish(t, "rate 0")
	env.publish(t, "rate 0.2")
	env.publish(t, "rate 11")
	env.publish(t, "rate abc")
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, "0.5", env.rdb.HGet(ctx, StateKey, "rate_mb").Val())

	env.publish(t, "rate 5")
	require.Eventually(t, func() bool {
		return env.rdb.HGet(ctx, StateKey, "rate_mb").Val() == "5"
	}, 2*time.Second, 20*time.Millisecond)

	// Stop cancels the lock-holding instance promptly.
	env.publish(t, "stop")
	stopped := env.waitHashStatus(t, StateKey, StatusStopped)
	require.Equal(t, StatusStopped, stopped["status"])

	// Resume at 10 MB/s completes cleanly.
	env.publish(t, "start 10")
	done := env.waitHashStatus(t, StateKey, StatusDone)
	require.Equal(t, "6", done["non_empty"])
	require.Equal(t, 6, env.client.StorageFileHash.Query().CountX(ctx))
}

func TestLockRenewalFailureAbortsTask(t *testing.T) {
	env := newMigratorEnv(t)
	m := NewFileHashesMigrator(zap.NewNop(), env.dx, env.rdb, env.dl)
	m.BatchSize = 1
	m.LockExpiry = 500 * time.Millisecond
	m.LockRenewInterval = 150 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = m.Start(ctx) }()
	<-m.Ready()
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = m.Stop(stopCtx)
		cancel()
	})

	bigVal := fmt.Sprintf(`{"blob":"%s"}`, strings.Repeat("y", 512<<10))
	for i := range 4 {
		env.insertLegacyFull(t, fmt.Sprintf("slow_%d", i), "'"+bigVal+"'")
	}

	env.publish(t, "start 0.5")
	env.waitHashStatus(t, StateKey, StatusRunning)

	// Delete the lock key in Redis so the next renewal fails.
	require.NoError(t, env.rdb.Del(context.Background(), LockKey).Err())

	errState := env.waitHashStatus(t, StateKey, StatusError)
	require.Contains(t, errState["last_error"], "lock renewal failed")
}

func TestCorruptLegacyRowSkippedAndCountedAsFailed(t *testing.T) {
	env := newMigratorEnv(t)
	env.startMigrator(t, 2)
	ctx := context.Background()

	idGood1 := env.insertLegacyFull(t, "ok1", `'{"a":"1"}'`)
	idBad := env.insertLegacyFull(t, "bad", `'{not-valid-json'`)
	idGood2 := env.insertLegacyFull(t, "ok2", `'{"b":"2"}'`)

	env.publish(t, "start 10")
	done := env.waitHashStatus(t, StateKey, StatusDone)
	require.Equal(t, "2", done["written"])
	require.Equal(t, "1", done["failed"])
	require.Equal(t, strconv.Itoa(idBad), done["failed_ids"])
	require.Contains(t, done["last_error"], strconv.Itoa(idBad))
	require.Equal(t, 2, env.client.StorageFileHash.Query().CountX(ctx))
	_ = idGood1
	_ = idGood2
}

func TestVerifyReportsUnknownCountMatchBeforeDone(t *testing.T) {
	env := newMigratorEnv(t)
	env.startMigrator(t, 1)
	ctx := context.Background()

	id1 := env.insertLegacyFull(t, "os1", `'{"a":"1"}'`)
	id2 := env.insertLegacyFull(t, "os2", `'{"b":"2"}'`)

	// Simulate an incomplete stopped backfill.
	require.NoError(t, env.rdb.HSet(ctx, StateKey,
		"status", StatusStopped,
		"max_id", id2,
		"cursor", id1,
		"non_empty", 1,
		"written", 1,
		"skipped", 0,
		"failed", 0,
	).Err())

	env.publish(t, "verify 10")
	v := env.waitHashStatus(t, VerifyKey, StatusDone)
	require.Equal(t, StatusStopped, v["state_status"])
	require.Equal(t, "unknown", v["count_match"])
}

func TestStopDoesNotFalseTriggerLockRenewalFailure(t *testing.T) {
	env := newMigratorEnv(t)
	m := NewFileHashesMigrator(zap.NewNop(), env.dx, env.rdb, env.dl)
	m.BatchSize = 1
	m.LockExpiry = 2 * time.Second
	m.LockRenewInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = m.Start(ctx) }()
	<-m.Ready()
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = m.Stop(stopCtx)
		cancel()
	})

	bigVal := fmt.Sprintf(`{"blob":"%s"}`, strings.Repeat("z", 256<<10))
	for i := range 6 {
		env.insertLegacyFull(t, fmt.Sprintf("r_%d", i), "'"+bigVal+"'")
	}

	// Repeat start/stop several times under aggressive 5ms renewal to exercise the cancel race.
	for range 5 {
		env.publish(t, "start 0.5")
		env.waitHashStatus(t, StateKey, StatusRunning)
		time.Sleep(15 * time.Millisecond)
		env.publish(t, "stop")
		stopped := env.waitHashStatus(t, StateKey, StatusStopped)
		require.Empty(t, stopped["last_error"])
		// Lock must have been released cleanly (not left for TTL expiry).
		require.Eventually(t, func() bool {
			return env.rdb.Exists(context.Background(), LockKey).Val() == 0
		}, time.Second, 10*time.Millisecond)
	}
}
