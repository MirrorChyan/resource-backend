package repo

import (
	"context"
	"database/sql"
	"errors"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/storagefilehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	FileHashesReadNew         = "new"
	FileHashesReadFallbackOld = "fallback_old"
	FileHashesReadMiss        = "miss"
	FileHashesReadError       = "error"
)

// storageColumnsWithoutFileHashes is derived from ent's generated column list so newly
// added Storage fields are never silently omitted from Select queries.
var storageColumnsWithoutFileHashes = func() []string {
	cols := make([]string, 0, len(storage.Columns))
	for _, c := range storage.Columns {
		if c != storage.FieldFileHashes {
			cols = append(cols, c)
		}
	}
	return cols
}()

var fileHashesReadsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "resource_file_hashes_reads_total",
	Help: "Total reads of storage file hashes, by outcome.",
}, []string{"result"})

func init() {
	for _, result := range []string{
		FileHashesReadNew,
		FileHashesReadFallbackOld,
		FileHashesReadMiss,
		FileHashesReadError,
	} {
		fileHashesReadsTotal.WithLabelValues(result)
	}
	_ = prometheus.Register(fileHashesReadsTotal)
}

// FileHashesReadsCounter returns the Prometheus counter for file hash read outcomes.
func FileHashesReadsCounter() *prometheus.CounterVec {
	return fileHashesReadsTotal
}

// SaveFileHashes inserts the compressed file hashes row for storageID inside tx.
// Empty hashes are intentionally not stored: absence of a row means "no file hashes".
func (r *Storage) SaveFileHashes(ctx context.Context, tx *ent.Tx, storageID int, encoded *filehash.EncodedHashes) error {
	if encoded == nil || len(encoded.Data) == 0 || encoded.Entries == 0 {
		return nil
	}
	return tx.StorageFileHash.Create().
		SetStorageID(storageID).
		SetFormat(encoded.Format).
		SetData(encoded.Data).
		SetRawSize(encoded.RawSize).
		SetEntries(encoded.Entries).
		Exec(ctx)
}

// GetFileHashes loads and decodes the file hashes of storageID: new table first,
// falling back to the legacy storages.file_hashes column when no row exists in the
// new table. Corrupted data returns an error rather than nil.
func (r *Storage) GetFileHashes(ctx context.Context, storageID int) (map[string]string, error) {
	_, hashes, err := r.getFileHashesWithEncoded(ctx, storageID)
	return hashes, err
}

func (r *Storage) getFileHashesWithEncoded(ctx context.Context, storageID int) (*filehash.EncodedHashes, map[string]string, error) {
	row, err := r.db.StorageFileHash.Query().
		Where(storagefilehash.StorageID(storageID)).
		Only(ctx)
	if err == nil {
		hashes, err := filehash.DecodeHashes(row.Format, row.Data)
		if err != nil {
			fileHashesReadsTotal.WithLabelValues(FileHashesReadError).Inc()
			return nil, nil, err
		}
		fileHashesReadsTotal.WithLabelValues(FileHashesReadNew).Inc()
		return &filehash.EncodedHashes{
			Format:  row.Format,
			Data:    row.Data,
			RawSize: row.RawSize,
			Entries: row.Entries,
		}, hashes, nil
	}
	if !ent.IsNotFound(err) {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadError).Inc()
		return nil, nil, err
	}

	var raw sql.NullString
	err = r.dx.QueryRowContext(ctx, "SELECT file_hashes FROM storages WHERE id = ?", storageID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadMiss).Inc()
		return nil, nil, nil
	}
	if err != nil {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadError).Inc()
		return nil, nil, err
	}

	hashes, _, err := filehash.ParseRawHashesJSON([]byte(raw.String), raw.Valid)
	if err != nil {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadError).Inc()
		return nil, nil, err
	}
	if len(hashes) == 0 {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadMiss).Inc()
		return nil, nil, nil
	}

	encoded, err := filehash.EncodeHashes(hashes)
	if err != nil {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadError).Inc()
		return nil, nil, err
	}
	fileHashesReadsTotal.WithLabelValues(FileHashesReadFallbackOld).Inc()
	return encoded, hashes, nil
}
