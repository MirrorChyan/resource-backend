package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"entgo.io/ent/dialect/sql/sqlgraph"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/go-redsync/redsync/v4"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	Channel   = "migrate:file_hashes"
	LockKey   = "migrate:file_hashes:lock"
	StateKey  = "migrate:file_hashes:state"
	StatsKey  = "migrate:file_hashes:stats"
	VerifyKey = "migrate:file_hashes:verify"

	DefaultRateMB = 2.0
	MinRateMB     = 0.5
	MaxRateMB     = 10.0

	DefaultBatchSize  = 20
	DefaultSampleSize = 500

	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusStopped = "stopped"
	StatusDone    = "done"
	StatusError   = "error"
)

const (
	defaultLockExpiry          = 30 * time.Second
	defaultLockRenewInterval   = 10 * time.Second
	defaultProgressLogInterval = 30 * time.Second
	redisOpTimeout             = 3 * time.Second
	maxRecordedFailedIDs       = 20
)

// FileHashesMigrator runs manual backfill, sampling stats, and verification for
// moving storages.file_hashes into storage_file_hashes. Operations are triggered via
// Redis Pub/Sub and report progress in Redis Hashes.
type FileHashesMigrator struct {
	logger *zap.Logger
	dx     *sqlx.DB
	rdb    *redis.Client
	sync   *redsync.Redsync

	LockExpiry          time.Duration
	LockRenewInterval   time.Duration
	ProgressLogInterval time.Duration
	BatchSize           int
	SampleSize          int

	readyOnce sync.Once
	readyCh   chan struct{}

	mu         sync.Mutex
	loopCancel context.CancelFunc
	loopDone   chan struct{}
	pubsub     *redis.PubSub

	// active* fields are non-nil only while this instance holds the distributed lock.
	activeStop       context.CancelFunc
	activeLimiter    *byteRateLimiter
	activeIsBackfill bool
	activeDone       chan struct{}
	wg               sync.WaitGroup
}

func NewFileHashesMigrator(logger *zap.Logger, dx *sqlx.DB, rdb *redis.Client, dl *redsync.Redsync) *FileHashesMigrator {
	return &FileHashesMigrator{
		logger:              logger,
		dx:                  dx,
		rdb:                 rdb,
		sync:                dl,
		LockExpiry:          defaultLockExpiry,
		LockRenewInterval:   defaultLockRenewInterval,
		ProgressLogInterval: defaultProgressLogInterval,
		BatchSize:           DefaultBatchSize,
		SampleSize:          DefaultSampleSize,
		readyCh:             make(chan struct{}),
	}
}

// Ready is closed once the Pub/Sub subscription has been established.
func (m *FileHashesMigrator) Ready() <-chan struct{} {
	return m.readyCh
}

// Start subscribes to Channel and dispatches commands until Stop is called or ctx ends.
func (m *FileHashesMigrator) Start(ctx context.Context) error {
	loopCtx, cancel := context.WithCancel(ctx)
	loopDone := make(chan struct{})
	sub := m.rdb.Subscribe(loopCtx, Channel)

	m.mu.Lock()
	m.loopCancel = cancel
	m.loopDone = loopDone
	m.pubsub = sub
	m.mu.Unlock()

	defer func() {
		_ = sub.Close()
		cancel()
		close(loopDone)
	}()

	if _, err := sub.Receive(loopCtx); err != nil {
		if loopCtx.Err() != nil {
			return nil
		}
		m.logger.Error("failed to subscribe file hashes migration channel", zap.Error(err))
		// Keep running so a transient Redis outage during startup does not kill the service.
	}
	m.readyOnce.Do(func() { close(m.readyCh) })

	for {
		msg, err := sub.ReceiveMessage(loopCtx)
		if err != nil {
			if loopCtx.Err() != nil {
				return nil
			}
			m.logger.Warn("file hashes migration pubsub receive failed", zap.Error(err))
			select {
			case <-time.After(time.Second):
			case <-loopCtx.Done():
				return nil
			}
			continue
		}
		m.handleMessage(loopCtx, msg.Payload)
	}
}

// Stop stops the Pub/Sub listener, cancels any running migration task, and waits for it
// to flush its state and release its distributed lock.
func (m *FileHashesMigrator) Stop(ctx context.Context) error {
	m.mu.Lock()
	loopCancel := m.loopCancel
	sub := m.pubsub
	activeStop := m.activeStop
	activeDone := m.activeDone
	loopDone := m.loopDone
	m.mu.Unlock()

	if loopCancel != nil {
		loopCancel()
	}
	if sub != nil {
		_ = sub.Close()
	}
	if activeStop != nil {
		activeStop()
	}
	if activeDone != nil {
		select {
		case <-activeDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if loopDone != nil {
		select {
		case <-loopDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	wgDone := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(wgDone)
	}()
	select {
	case <-wgDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (m *FileHashesMigrator) handleMessage(ctx context.Context, payload string) {
	fields := strings.Fields(payload)
	if len(fields) == 0 {
		return
	}
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "start":
		var rateOverride *float64
		if len(args) > 0 {
			mb, ok := parseRateMB(args[0])
			if !ok {
				m.logger.Warn("ignoring file hashes start with out-of-range rate", zap.String("payload", payload))
				return
			}
			rateOverride = &mb
		}
		m.spawn(func() { m.runBackfill(ctx, rateOverride) })
	case "stop":
		m.handleStop()
	case "rate":
		if len(args) == 0 {
			m.logger.Warn("ignoring file hashes rate command without value")
			return
		}
		m.handleRate(args[0])
	case "stats":
		sampleSize := m.sampleSize()
		if len(args) > 0 {
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				m.logger.Warn("ignoring file hashes stats with invalid sample size", zap.String("payload", payload))
				return
			}
			sampleSize = n
		}
		m.spawn(func() { m.runStats(ctx, sampleSize) })
	case "verify":
		sampleSize := m.sampleSize()
		if len(args) > 0 {
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 {
				m.logger.Warn("ignoring file hashes verify with invalid sample size", zap.String("payload", payload))
				return
			}
			sampleSize = n
		}
		m.spawn(func() { m.runVerify(ctx, sampleSize) })
	case "reset":
		m.spawn(func() { m.runReset(ctx) })
	default:
		m.logger.Warn("ignoring unknown file hashes migration command", zap.String("payload", payload))
	}
}

func (m *FileHashesMigrator) spawn(fn func()) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		fn()
	}()
}

func (m *FileHashesMigrator) handleStop() {
	m.mu.Lock()
	cancel := m.activeStop
	m.mu.Unlock()
	if cancel == nil {
		return
	}
	m.logger.Info("stopping active file hashes migration task")
	cancel()
}

func (m *FileHashesMigrator) handleRate(raw string) {
	mb, ok := parseRateMB(raw)
	if !ok {
		m.logger.Warn("ignoring out-of-range file hashes migration rate", zap.String("rate", raw))
		return
	}
	m.mu.Lock()
	limiter := m.activeLimiter
	isBackfill := m.activeIsBackfill
	m.mu.Unlock()
	if limiter == nil {
		return
	}
	limiter.SetRateMB(mb)
	m.logger.Info("updated file hashes migration rate", zap.Float64("rate_mb", mb))
	if isBackfill {
		rctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
		defer cancel()
		_ = m.rdb.HSet(rctx, StateKey,
			"rate_mb", formatFloat(mb),
			"updated_at", nowRFC3339(),
		).Err()
	}
}

func (m *FileHashesMigrator) lockExpiry() time.Duration {
	if m.LockExpiry > 0 {
		return m.LockExpiry
	}
	return defaultLockExpiry
}

func (m *FileHashesMigrator) lockRenewInterval() time.Duration {
	if m.LockRenewInterval > 0 {
		return m.LockRenewInterval
	}
	return defaultLockRenewInterval
}

func (m *FileHashesMigrator) batchSize() int {
	if m.BatchSize > 0 {
		return m.BatchSize
	}
	return DefaultBatchSize
}

func (m *FileHashesMigrator) sampleSize() int {
	if m.SampleSize > 0 {
		return m.SampleSize
	}
	return DefaultSampleSize
}

func (m *FileHashesMigrator) progressLogInterval() time.Duration {
	if m.ProgressLogInterval > 0 {
		return m.ProgressLogInterval
	}
	return defaultProgressLogInterval
}

type taskLock struct {
	ctx      context.Context
	cancel   context.CancelFunc
	lockLost *atomic.Bool
	limiter  *byteRateLimiter
	release  func()
}

// acquireTaskLock acquires the shared distributed lock non-blockingly (WithTries(1)) and
// starts heartbeat renewal. If renewal fails, task.ctx is cancelled immediately and
// lockLost is set to true.
func (m *FileHashesMigrator) acquireTaskLock(parent context.Context, isBackfill bool, initialRateMB float64) (*taskLock, bool) {
	mutex := m.sync.NewMutex(LockKey,
		redsync.WithExpiry(m.lockExpiry()),
		redsync.WithTries(1),
	)
	if err := mutex.LockContext(parent); err != nil {
		m.logger.Info("file hashes migration lock busy, ignoring command", zap.Error(err))
		return nil, false
	}

	taskCtx, cancel := context.WithCancel(parent)
	lockLost := &atomic.Bool{}
	limiter := newByteRateLimiter(initialRateMB)
	done := make(chan struct{})

	m.mu.Lock()
	m.activeStop = cancel
	m.activeLimiter = limiter
	m.activeIsBackfill = isBackfill
	m.activeDone = done
	m.mu.Unlock()

	renewStop := make(chan struct{})
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(m.lockRenewInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if taskCtx.Err() != nil {
					return
				}
				select {
				case <-renewStop:
					return
				default:
				}
				ok, err := mutex.ExtendContext(taskCtx)
				if taskCtx.Err() != nil {
					return
				}
				if err != nil || !ok {
					m.logger.Error("file hashes migration lock renewal failed, aborting task",
						zap.Bool("ok", ok),
						zap.Error(err),
					)
					lockLost.Store(true)
					cancel()
					return
				}
			case <-renewStop:
				return
			case <-taskCtx.Done():
				return
			}
		}
	}()

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(renewStop)
			<-renewDone
			cancel()

			m.mu.Lock()
			m.activeStop = nil
			m.activeLimiter = nil
			m.activeIsBackfill = false
			m.activeDone = nil
			m.mu.Unlock()

			if !lockLost.Load() {
				uctx, ucancel := context.WithTimeout(context.Background(), redisOpTimeout)
				defer ucancel()
				_, _ = mutex.UnlockContext(uctx)
			}
			close(done)
		})
	}

	return &taskLock{
		ctx:      taskCtx,
		cancel:   cancel,
		lockLost: lockLost,
		limiter:  limiter,
		release:  release,
	}, true
}

func (m *FileHashesMigrator) runReset(parent context.Context) {
	lock, ok := m.acquireTaskLock(parent, false, DefaultRateMB)
	if !ok {
		return
	}
	defer lock.release()

	rctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	if err := m.rdb.Del(rctx, StateKey).Err(); err != nil {
		m.logger.Error("failed to reset file hashes migration state", zap.Error(err))
		return
	}
	m.logger.Info("file hashes migration state reset")
}

func (m *FileHashesMigrator) runBackfill(parent context.Context, rateOverride *float64) {
	initialRate := DefaultRateMB
	if rateOverride != nil {
		initialRate = *rateOverride
	}
	lock, ok := m.acquireTaskLock(parent, true, initialRate)
	if !ok {
		return
	}
	defer lock.release()

	ctx := lock.ctx

	state, err := m.rdb.HGetAll(ctx, StateKey).Result()
	if err != nil {
		m.logger.Error("failed to load file hashes migration state", zap.Error(err))
		return
	}
	if state["status"] == StatusDone {
		m.logger.Info("file hashes migration already done, send reset to rerun")
		return
	}

	var (
		maxID        int
		cursor       int
		written      int64
		skipped      int64
		empty        int64
		failed       int64
		failedIDs    []string
		lastRowErr   string
		bytesRead    int64
		bytesWritten int64
		startedAt    string
		rateMB       = initialRate
	)

	if rawMaxID, hasMax := state["max_id"]; hasMax && rawMaxID != "" {
		maxID, _ = strconv.Atoi(rawMaxID)
		cursor, _ = strconv.Atoi(state["cursor"])
		written, _ = strconv.ParseInt(state["written"], 10, 64)
		skipped, _ = strconv.ParseInt(state["skipped"], 10, 64)
		empty, _ = strconv.ParseInt(state["empty"], 10, 64)
		failed, _ = strconv.ParseInt(state["failed"], 10, 64)
		if rawIDs := state["failed_ids"]; rawIDs != "" {
			failedIDs = strings.Split(rawIDs, ",")
		}
		bytesRead, _ = strconv.ParseInt(state["bytes_read"], 10, 64)
		bytesWritten, _ = strconv.ParseInt(state["bytes_written"], 10, 64)
		startedAt = state["started_at"]
		if startedAt == "" {
			startedAt = nowRFC3339()
		}
		if rateOverride == nil {
			if savedRate, ok := parseRateMB(state["rate_mb"]); ok {
				rateMB = savedRate
				lock.limiter.SetRateMB(rateMB)
			}
		}
	} else {
		if err := m.dx.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM storages").Scan(&maxID); err != nil {
			m.writeStateError(fmt.Errorf("query max storage id: %w", err))
			return
		}
		startedAt = nowRFC3339()
	}

	writeState := func(status, lastErr string) {
		rctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
		defer cancel()
		_ = m.rdb.HSet(rctx, StateKey,
			"status", status,
			"max_id", maxID,
			"cursor", cursor,
			"non_empty", written+skipped,
			"empty", empty,
			"written", written,
			"skipped", skipped,
			"failed", failed,
			"failed_ids", strings.Join(failedIDs, ","),
			"bytes_read", bytesRead,
			"bytes_written", bytesWritten,
			"rate_mb", formatFloat(lock.limiter.RateMB()),
			"started_at", startedAt,
			"updated_at", nowRFC3339(),
			"last_error", lastErr,
		).Err()
	}

	writeState(StatusRunning, "")
	m.logger.Info("file hashes backfill started",
		zap.Int("max_id", maxID),
		zap.Int("cursor", cursor),
		zap.Float64("rate_mb", rateMB),
	)

	batchSize := m.batchSize()
	logInterval := m.progressLogInterval()
	lastLogAt := time.Now()
	for {
		if ctx.Err() != nil {
			break
		}

		ids, err := m.fetchFullStorageIDBatch(ctx, cursor, maxID, batchSize)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			m.logger.Error("failed to query storage id batch", zap.Error(err))
			writeState(StatusError, err.Error())
			return
		}
		if len(ids) == 0 {
			writeState(StatusDone, lastRowErr)
			m.logger.Info("file hashes backfill completed",
				zap.Int("max_id", maxID),
				zap.Int64("written", written),
				zap.Int64("skipped", skipped),
				zap.Int64("empty", empty),
				zap.Int64("failed", failed),
				zap.Int64("bytes_read", bytesRead),
				zap.Int64("bytes_written", bytesWritten),
			)
			return
		}

		existing, err := m.fetchExistingHashStorageIDs(ctx, ids)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			m.logger.Error("failed to check existing file hashes", zap.Error(err))
			writeState(StatusError, err.Error())
			return
		}

		missingIDs := make([]int, 0, len(ids))
		for _, id := range ids {
			if !existing[id] {
				missingIDs = append(missingIDs, id)
			}
		}

		var rawByID map[int]sql.NullString
		if len(missingIDs) > 0 {
			rawByID, err = m.fetchRawFileHashes(ctx, missingIDs)
			if err != nil {
				if ctx.Err() != nil {
					break
				}
				m.logger.Error("failed to read legacy file_hashes batch", zap.Error(err))
				writeState(StatusError, err.Error())
				return
			}
		}

		var (
			lastID            = cursor
			batchWritten      int64
			batchSkipped      int64
			batchEmpty        int64
			batchFailed       int64
			batchFailedIDs    []string
			batchBytesRead    int64
			batchBytesWritten int64
			rowErr            error
		)

		for _, id := range ids {
			if ctx.Err() != nil {
				break
			}
			if existing[id] {
				batchSkipped++
				lastID = id
				continue
			}

			raw := rawByID[id]
			readN := 0
			if raw.Valid {
				readN = len(raw.String)
			}
			if readN > 0 {
				if err := lock.limiter.Wait(ctx, readN); err != nil {
					break
				}
			}

			hashes, _, err := filehash.ParseRawHashesJSON([]byte(raw.String), raw.Valid)
			batchBytesRead += int64(readN)
			if err != nil {
				m.logger.Error("skipping storage with unparseable legacy file_hashes",
					zap.Int("storage_id", id),
					zap.Error(err),
				)
				batchFailed++
				batchFailedIDs = append(batchFailedIDs, strconv.Itoa(id))
				lastRowErr = fmt.Sprintf("parse file_hashes for storage %d: %v", id, err)
				lastID = id
				continue
			}

			if len(hashes) == 0 {
				batchEmpty++
				lastID = id
				continue
			}

			encoded, err := filehash.EncodeHashes(hashes)
			if err != nil {
				m.logger.Error("skipping storage whose file_hashes failed to encode",
					zap.Int("storage_id", id),
					zap.Error(err),
				)
				batchFailed++
				batchFailedIDs = append(batchFailedIDs, strconv.Itoa(id))
				lastRowErr = fmt.Sprintf("encode file_hashes for storage %d: %v", id, err)
				lastID = id
				continue
			}

			inserted, err := m.insertFileHashTx(ctx, id, encoded)
			if err != nil {
				if ctx.Err() != nil {
					break
				}
				rowErr = fmt.Errorf("insert storage_file_hashes for storage %d: %w", id, err)
				break
			}
			if inserted {
				batchWritten++
				batchBytesWritten += int64(len(encoded.Data))
			} else {
				batchSkipped++
			}
			lastID = id
		}

		if lastID > cursor {
			cursor = lastID
			written += batchWritten
			skipped += batchSkipped
			empty += batchEmpty
			failed += batchFailed
			for _, fid := range batchFailedIDs {
				if len(failedIDs) < maxRecordedFailedIDs {
					failedIDs = append(failedIDs, fid)
				}
			}
			bytesRead += batchBytesRead
			bytesWritten += batchBytesWritten
			writeState(StatusRunning, lastRowErr)
			if time.Since(lastLogAt) >= logInterval {
				lastLogAt = time.Now()
				m.logger.Info("file hashes backfill progress",
					zap.Int("cursor", cursor),
					zap.Int("max_id", maxID),
					zap.Int64("written", written),
					zap.Int64("skipped", skipped),
					zap.Int64("empty", empty),
					zap.Int64("failed", failed),
					zap.Int64("bytes_read", bytesRead),
					zap.Int64("bytes_written", bytesWritten),
				)
			}
		}

		if rowErr != nil {
			m.logger.Error("file hashes backfill failed", zap.Error(rowErr))
			writeState(StatusError, rowErr.Error())
			return
		}
		if ctx.Err() != nil {
			break
		}
	}

	if lock.lockLost.Load() {
		writeState(StatusError, "distributed lock renewal failed")
		return
	}
	writeState(StatusStopped, "")
	m.logger.Info("file hashes backfill stopped",
		zap.Int("cursor", cursor),
		zap.Int("max_id", maxID),
		zap.Int64("written", written),
		zap.Int64("skipped", skipped),
		zap.Int64("empty", empty),
	)
}

func (m *FileHashesMigrator) writeStateError(err error) {
	rctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	_ = m.rdb.HSet(rctx, StateKey,
		"status", StatusError,
		"last_error", err.Error(),
		"updated_at", nowRFC3339(),
	).Err()
}

func (m *FileHashesMigrator) fetchFullStorageIDBatch(ctx context.Context, cursor, maxID, limit int) ([]int, error) {
	rows, err := m.dx.QueryContext(ctx,
		"SELECT id FROM storages WHERE id > ? AND id <= ? AND update_type = 'full' ORDER BY id ASC LIMIT ?",
		cursor, maxID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int, 0, limit)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (m *FileHashesMigrator) fetchExistingHashStorageIDs(ctx context.Context, ids []int) (map[int]bool, error) {
	query, args, err := sqlx.In("SELECT storage_id FROM storage_file_hashes WHERE storage_id IN (?)", ids)
	if err != nil {
		return nil, err
	}
	rows, err := m.dx.QueryContext(ctx, m.dx.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	existing := make(map[int]bool, len(ids))
	for rows.Next() {
		var sid int
		if err := rows.Scan(&sid); err != nil {
			return nil, err
		}
		existing[sid] = true
	}
	return existing, rows.Err()
}

func (m *FileHashesMigrator) fetchRawFileHashes(ctx context.Context, ids []int) (map[int]sql.NullString, error) {
	query, args, err := sqlx.In("SELECT id, file_hashes FROM storages WHERE id IN (?)", ids)
	if err != nil {
		return nil, err
	}
	rows, err := m.dx.QueryContext(ctx, m.dx.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int]sql.NullString, len(ids))
	for rows.Next() {
		var (
			id  int
			raw sql.NullString
		)
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		result[id] = raw
	}
	return result, rows.Err()
}

// insertFileHashTx inserts one compressed file_hashes row in a short transaction.
// Returns (false, nil) if the row already existed or raced on the unique constraint.
func (m *FileHashesMigrator) insertFileHashTx(ctx context.Context, storageID int, encoded *filehash.EncodedHashes) (bool, error) {
	tx, err := m.dx.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM storage_file_hashes WHERE storage_id = ?", storageID).Scan(&exists)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO storage_file_hashes (storage_id, format, data, raw_size, entries) VALUES (?, ?, ?, ?, ?)",
		storageID, encoded.Format, encoded.Data, encoded.RawSize, encoded.Entries,
	)
	if err != nil {
		if sqlgraph.IsUniqueConstraintError(err) {
			return false, nil
		}
		return false, err
	}
	if err := tx.Commit(); err != nil {
		if sqlgraph.IsUniqueConstraintError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (m *FileHashesMigrator) runStats(parent context.Context, sampleSize int) {
	lock, ok := m.acquireTaskLock(parent, false, DefaultRateMB)
	if !ok {
		return
	}
	defer lock.release()

	ctx := lock.ctx
	startedAt := nowRFC3339()
	m.writeHashResult(StatsKey, map[string]any{
		"status":     StatusRunning,
		"started_at": startedAt,
		"updated_at": startedAt,
		"last_error": "",
	})

	var (
		minID int
		maxID int
	)
	if err := m.dx.QueryRowContext(ctx, "SELECT COALESCE(MIN(id), 0), COALESCE(MAX(id), 0) FROM storages WHERE update_type = 'full'").Scan(&minID, &maxID); err != nil {
		m.writeHashResult(StatsKey, map[string]any{
			"status":     StatusError,
			"last_error": err.Error(),
			"updated_at": nowRFC3339(),
		})
		return
	}

	var (
		sampled         int
		nonEmpty        int
		sqlNullCount    int
		jsonNullCount   int
		emptyObjCount   int
		parseErrors     int
		totalEntries    int64
		rawBytes        int64
		zstdDefaultSize int64
		zstdBetterSize  int64
		zstdDefaultDur  time.Duration
		zstdBetterDur   time.Duration
	)

	seen := make(map[int]bool, sampleSize)
	if minID > 0 && maxID >= minID {
		maxAttempts := max(sampleSize*5, 100)
		span := maxID - minID + 1
		for attempt := 0; attempt < maxAttempts && sampled < sampleSize; attempt++ {
			if ctx.Err() != nil {
				break
			}
			probe := minID + rand.IntN(span)
			id, raw, found, err := m.probeFullStorage(ctx, probe, maxID)
			if err != nil {
				if ctx.Err() != nil {
					break
				}
				m.writeHashResult(StatsKey, map[string]any{
					"status":     StatusError,
					"last_error": err.Error(),
					"updated_at": nowRFC3339(),
				})
				return
			}
			if !found || seen[id] {
				continue
			}
			seen[id] = true

			if raw.Valid && len(raw.String) > 0 {
				if err := lock.limiter.Wait(ctx, len(raw.String)); err != nil {
					break
				}
			}

			hashes, kind, err := filehash.ParseRawHashesJSON([]byte(raw.String), raw.Valid)
			sampled++
			if err != nil {
				parseErrors++
				continue
			}
			switch kind {
			case filehash.RawHashSQLNull:
				sqlNullCount++
			case filehash.RawHashJSONNull:
				jsonNullCount++
			case filehash.RawHashEmptyObject:
				emptyObjCount++
			case filehash.RawHashNonEmpty:
				nonEmpty++
				totalEntries += int64(len(hashes))
				canonical, err := filehash.MarshalHashes(hashes)
				if err != nil {
					parseErrors++
					continue
				}
				rawBytes += int64(len(canonical))

				t0 := time.Now()
				defBuf := filehash.CompressRawJSON(canonical, false)
				zstdDefaultDur += time.Since(t0)
				zstdDefaultSize += int64(len(defBuf))

				t1 := time.Now()
				betBuf := filehash.CompressRawJSON(canonical, true)
				zstdBetterDur += time.Since(t1)
				zstdBetterSize += int64(len(betBuf))
			}
		}
	}

	status := StatusDone
	lastErr := ""
	if lock.lockLost.Load() {
		status = StatusError
		lastErr = "distributed lock renewal failed"
	} else if ctx.Err() != nil {
		status = StatusStopped
	}

	defaultRatio := 0.0
	if zstdDefaultSize > 0 {
		defaultRatio = float64(rawBytes) / float64(zstdDefaultSize)
	}
	betterRatio := 0.0
	if zstdBetterSize > 0 {
		betterRatio = float64(rawBytes) / float64(zstdBetterSize)
	}

	m.writeHashResult(StatsKey, map[string]any{
		"status":             status,
		"sampled":            sampled,
		"non_empty":          nonEmpty,
		"sql_null":           sqlNullCount,
		"json_null":          jsonNullCount,
		"empty_object":       emptyObjCount,
		"parse_errors":       parseErrors,
		"total_entries":      totalEntries,
		"raw_bytes":          rawBytes,
		"zstd_default_bytes": zstdDefaultSize,
		"zstd_default_ratio": fmt.Sprintf("%.2f", defaultRatio),
		"zstd_default_ms":    zstdDefaultDur.Milliseconds(),
		"zstd_better_bytes":  zstdBetterSize,
		"zstd_better_ratio":  fmt.Sprintf("%.2f", betterRatio),
		"zstd_better_ms":     zstdBetterDur.Milliseconds(),
		"updated_at":         nowRFC3339(),
		"last_error":         lastErr,
	})
	m.logger.Info("file hashes stats finished",
		zap.String("status", status),
		zap.Int("sampled", sampled),
		zap.Int("non_empty", nonEmpty),
		zap.Int64("raw_bytes", rawBytes),
		zap.Int64("zstd_default_bytes", zstdDefaultSize),
		zap.Int64("zstd_better_bytes", zstdBetterSize),
	)
}

func (m *FileHashesMigrator) runVerify(parent context.Context, sampleSize int) {
	lock, ok := m.acquireTaskLock(parent, false, DefaultRateMB)
	if !ok {
		return
	}
	defer lock.release()

	ctx := lock.ctx
	startedAt := nowRFC3339()
	m.writeHashResult(VerifyKey, map[string]any{
		"status":     StatusRunning,
		"started_at": startedAt,
		"updated_at": startedAt,
		"last_error": "",
	})

	state, err := m.rdb.HGetAll(ctx, StateKey).Result()
	if err != nil {
		m.writeHashResult(VerifyKey, map[string]any{
			"status":     StatusError,
			"last_error": err.Error(),
			"updated_at": nowRFC3339(),
		})
		return
	}

	var (
		maxID         int
		stateStatus   = state["status"]
		stateNonEmpty int64
		stateFailed   int64
		hasStateBound bool
	)
	if rawMax, ok := state["max_id"]; ok && rawMax != "" {
		maxID, _ = strconv.Atoi(rawMax)
		stateNonEmpty, _ = strconv.ParseInt(state["non_empty"], 10, 64)
		stateFailed, _ = strconv.ParseInt(state["failed"], 10, 64)
		hasStateBound = true
	} else {
		if err := m.dx.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM storages").Scan(&maxID); err != nil {
			m.writeHashResult(VerifyKey, map[string]any{
				"status":     StatusError,
				"last_error": err.Error(),
				"updated_at": nowRFC3339(),
			})
			return
		}
	}

	var newTableCount int64
	if err := m.dx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM storage_file_hashes WHERE storage_id <= ?",
		maxID,
	).Scan(&newTableCount); err != nil {
		m.writeHashResult(VerifyKey, map[string]any{
			"status":     StatusError,
			"last_error": err.Error(),
			"updated_at": nowRFC3339(),
		})
		return
	}

	var (
		minFullID int
		maxFullID int
	)
	if err := m.dx.QueryRowContext(ctx,
		"SELECT COALESCE(MIN(id), 0), COALESCE(MAX(id), 0) FROM storages WHERE id <= ? AND update_type = 'full'",
		maxID,
	).Scan(&minFullID, &maxFullID); err != nil {
		m.writeHashResult(VerifyKey, map[string]any{
			"status":     StatusError,
			"last_error": err.Error(),
			"updated_at": nowRFC3339(),
		})
		return
	}

	var (
		sampled    int
		matched    int
		emptyOK    int
		missing    int
		mismatched int
		errCount   int
	)

	seen := make(map[int]bool, sampleSize)
	if minFullID > 0 && maxFullID >= minFullID {
		maxAttempts := max(sampleSize*5, 100)
		span := maxFullID - minFullID + 1
		for attempt := 0; attempt < maxAttempts && sampled < sampleSize; attempt++ {
			if ctx.Err() != nil {
				break
			}
			probe := minFullID + rand.IntN(span)
			id, raw, found, err := m.probeFullStorage(ctx, probe, maxFullID)
			if err != nil {
				if ctx.Err() != nil {
					break
				}
				m.writeHashResult(VerifyKey, map[string]any{
					"status":     StatusError,
					"last_error": err.Error(),
					"updated_at": nowRFC3339(),
				})
				return
			}
			if !found || seen[id] {
				continue
			}
			seen[id] = true

			if raw.Valid && len(raw.String) > 0 {
				if err := lock.limiter.Wait(ctx, len(raw.String)); err != nil {
					break
				}
			}

			oldHashes, _, err := filehash.ParseRawHashesJSON([]byte(raw.String), raw.Valid)
			sampled++
			if err != nil {
				errCount++
				continue
			}

			var (
				format uint8
				data   []byte
			)
			err = m.dx.QueryRowContext(ctx,
				"SELECT format, data FROM storage_file_hashes WHERE storage_id = ?",
				id,
			).Scan(&format, &data)
			if errors.Is(err, sql.ErrNoRows) {
				if len(oldHashes) == 0 {
					emptyOK++
				} else {
					missing++
				}
				continue
			}
			if err != nil {
				errCount++
				continue
			}

			if len(oldHashes) == 0 {
				mismatched++
				continue
			}
			newHashes, err := filehash.DecodeHashes(format, data)
			if err != nil {
				errCount++
				continue
			}
			if maps.Equal(oldHashes, newHashes) {
				matched++
			} else {
				mismatched++
			}
		}
	}

	status := StatusDone
	lastErr := ""
	if lock.lockLost.Load() {
		status = StatusError
		lastErr = "distributed lock renewal failed"
	} else if ctx.Err() != nil {
		status = StatusStopped
	}

	countMatch := "unknown"
	if hasStateBound && stateStatus == StatusDone {
		countMatch = strconv.FormatBool(newTableCount == stateNonEmpty)
	}

	m.writeHashResult(VerifyKey, map[string]any{
		"status":          status,
		"state_status":    stateStatus,
		"state_failed":    stateFailed,
		"max_id":          maxID,
		"new_table_count": newTableCount,
		"state_non_empty": stateNonEmpty,
		"count_match":     countMatch,
		"sampled":         sampled,
		"matched":         matched,
		"empty_ok":        emptyOK,
		"missing":         missing,
		"mismatched":      mismatched,
		"errors":          errCount,
		"updated_at":      nowRFC3339(),
		"last_error":      lastErr,
	})
	m.logger.Info("file hashes verify finished",
		zap.String("status", status),
		zap.Int("max_id", maxID),
		zap.Int64("new_table_count", newTableCount),
		zap.Int64("state_non_empty", stateNonEmpty),
		zap.Int("sampled", sampled),
		zap.Int("matched", matched),
		zap.Int("missing", missing),
		zap.Int("mismatched", mismatched),
		zap.Int("errors", errCount),
	)
}

func (m *FileHashesMigrator) probeFullStorage(ctx context.Context, probeID, maxID int) (int, sql.NullString, bool, error) {
	var (
		id  int
		raw sql.NullString
	)
	err := m.dx.QueryRowContext(ctx,
		"SELECT id, file_hashes FROM storages WHERE id >= ? AND id <= ? AND update_type = 'full' ORDER BY id ASC LIMIT 1",
		probeID, maxID,
	).Scan(&id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, sql.NullString{}, false, nil
	}
	if err != nil {
		return 0, sql.NullString{}, false, err
	}
	return id, raw, true, nil
}

func (m *FileHashesMigrator) writeHashResult(key string, fields map[string]any) {
	rctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	_ = m.rdb.HSet(rctx, key, fields).Err()
}

func parseRateMB(raw string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < MinRateMB || v > MaxRateMB {
		return 0, false
	}
	return v, true
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}

// byteRateLimiter paces reads at a dynamically adjustable MB/s rate and wakes
// immediately on context cancellation or rate changes.
type byteRateLimiter struct {
	mu     sync.Mutex
	rateMB float64
	notify chan struct{}
}

func newByteRateLimiter(rateMB float64) *byteRateLimiter {
	if rateMB < MinRateMB || rateMB > MaxRateMB {
		rateMB = DefaultRateMB
	}
	return &byteRateLimiter{
		rateMB: rateMB,
		notify: make(chan struct{}),
	}
}

func (l *byteRateLimiter) RateMB() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rateMB
}

func (l *byteRateLimiter) SetRateMB(rateMB float64) {
	l.mu.Lock()
	l.rateMB = rateMB
	close(l.notify)
	l.notify = make(chan struct{})
	l.mu.Unlock()
}

func (l *byteRateLimiter) Wait(ctx context.Context, n int) error {
	if n <= 0 {
		return ctx.Err()
	}
	remaining := float64(n)
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		bps := l.rateMB * 1024 * 1024
		ch := l.notify
		l.mu.Unlock()

		need := time.Duration(remaining / bps * float64(time.Second))
		if need <= 0 {
			return nil
		}
		start := time.Now()
		timer := time.NewTimer(need)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			return nil
		case <-ch:
			timer.Stop()
			elapsed := time.Since(start).Seconds()
			remaining -= elapsed * bps
		}
	}
	return nil
}
