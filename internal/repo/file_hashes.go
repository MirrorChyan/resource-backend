package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storagefilehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	FileHashesReadNew   = "new"
	FileHashesReadMiss  = "miss"
	FileHashesReadError = "error"
)

var fileHashesReadsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "resource_file_hashes_reads_total",
	Help: "Total reads of storage file hashes, by outcome.",
}, []string{"result"})

func init() {
	for _, result := range []string{
		FileHashesReadNew,
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

// GetFileHashes loads and decodes the file hashes of storageID. A storage without a row
// has no file hashes and gets nil; corrupted data returns an error rather than nil.
func (r *Storage) GetFileHashes(ctx context.Context, storageID int) (map[string]string, error) {
	_, hashes, err := r.getFileHashesWithEncoded(ctx, storageID)
	return hashes, err
}

func (r *Storage) getFileHashesWithEncoded(ctx context.Context, storageID int) (*filehash.EncodedHashes, map[string]string, error) {
	row, err := r.db.StorageFileHash.Query().
		Where(storagefilehash.StorageID(storageID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadMiss).Inc()
		return nil, nil, nil
	}
	if err != nil {
		fileHashesReadsTotal.WithLabelValues(FileHashesReadError).Inc()
		return nil, nil, err
	}
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
