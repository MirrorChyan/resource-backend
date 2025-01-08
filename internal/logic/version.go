package logic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"go.uber.org/zap"
)

type VersionLogic struct {
	logger *zap.Logger
	db     *ent.Client
}

func NewVersionLogic(logger *zap.Logger, db *ent.Client) *VersionLogic {
	return &VersionLogic{
		logger: logger,
		db:     db,
	}
}

type VersionNameExistsParam struct {
	ResourceID int
	Name       string
}

func (l *VersionLogic) NameExists(ctx context.Context, param VersionNameExistsParam) (bool, error) {
	return l.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(param.ResourceID))).
		Where(version.Name(param.Name)).
		Exist(ctx)
}

func (l *VersionLogic) GetLatest(ctx context.Context, resourceID int) (*ent.Version, error) {
	return l.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(resourceID))).
		Order(ent.Desc("number")).
		First(ctx)
}

type CreateVersionParam struct {
	ResourceID        int
	Name              string
	UploadArchivePath string
}

func (l *VersionLogic) Create(ctx context.Context, param CreateVersionParam) (*ent.Version, string, error) {
	tx, err := l.db.Tx(ctx)
	if err != nil {
		l.logger.Error("Failed to start transaction",
			zap.Error(err),
		)
		return nil, "", err
	}

	var number uint64
	latest, err := tx.Version.Query().
		Where(version.HasResourceWith(resource.ID(param.ResourceID))).
		Order(ent.Desc("number")).
		First(ctx)
	if ent.IsNotFound(err) {
		number = 1
	} else if err != nil {
		l.logger.Error("Failed to query latest version",
			zap.Error(err),
		)
		return nil, "", err
	}
	if latest != nil {
		number = latest.Number + 1
	}
	v, err := tx.Version.Create().
		SetResourceID(param.ResourceID).
		SetName(param.Name).
		SetNumber(number).
		Save(ctx)
	if err != nil {
		l.logger.Error("Failed to create version",
			zap.Error(err),
		)
		return l.createRollback(tx, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		l.logger.Error("Failed to get current working directory",
			zap.Error(err),
		)
		return l.createRollback(tx, err)
	}
	storageRootDir := filepath.Join(cwd, "storage")
	versionDir := filepath.Join(storageRootDir, strconv.Itoa(param.ResourceID), strconv.Itoa(v.ID))
	saveDir := filepath.Join(versionDir, "resource")
	if err := os.MkdirAll(saveDir, os.ModePerm); err != nil {
		l.logger.Error("Failed to create storage directory",
			zap.Error(err),
		)
		return l.createRollback(tx, err)
	}

	if strings.HasSuffix(param.UploadArchivePath, ".zip") {
		err = archive.UnpackZip(param.UploadArchivePath, saveDir)
	} else if strings.HasSuffix(param.UploadArchivePath, ".tar.gz") {
		err = archive.UnpackTarGz(param.UploadArchivePath, saveDir)
	} else {
		l.logger.Error("Unknown archive extension",
			zap.String("archive path", param.UploadArchivePath),
		)
		err = fmt.Errorf("Unknown archive extension")
		return l.createRollback(tx, err)
	}

	if err != nil {
		l.logger.Error("Failed to unpack file",
			zap.String("version name", param.Name),
			zap.Error(err),
		)
		return l.createRollbackRemoveSaveDir(tx, err, saveDir)
	}

	archivePath := filepath.Join(versionDir, "resource.zip")
	if strings.HasSuffix(param.UploadArchivePath, ".zip") {
		err = fileops.MoveFile(param.UploadArchivePath, archivePath)
		if err != nil {
			l.logger.Error("Failed to move archive file",
				zap.String("origin path", param.UploadArchivePath),
				zap.String("destination path", archivePath),
				zap.Error(err),
			)
			return l.createRollbackRemoveSaveDir(tx, err, saveDir)
		}
	} else {
		if err := os.Remove(param.UploadArchivePath); err != nil {
			l.logger.Error("Failed to remove temp file",
				zap.Error(err),
			)
			return l.createRollbackRemoveSaveDir(tx, err, saveDir)
		}
		err = archive.CompressToZip(saveDir, archivePath)
		if err != nil {
			l.logger.Error("Failed to compress to zip",
				zap.String("src dir", saveDir),
				zap.String("dst file", archivePath),
				zap.Error(err),
			)
			return l.createRollbackRemoveSaveDir(tx, err, saveDir)
		}

	}

	fileHashes, err := filehash.GetAll(saveDir)
	if err != nil {
		l.logger.Error("Failed to get file hashes",
			zap.String("version name", param.Name),
			zap.Error(err),
		)
		return l.createRollbackRemoveSaveDir(tx, err, saveDir)
	}
	v, err = tx.Version.UpdateOne(v).
		SetFileHashes(fileHashes).
		Save(ctx)
	if err != nil {
		l.logger.Error("Failed to add file hashes to version",
			zap.Error(err),
		)
		return l.createRollbackRemoveSaveDir(tx, err, saveDir)
	}

	err = tx.Commit()
	if err != nil {
		l.logger.Error("Failed to commit transaction",
			zap.Error(err),
		)
		return nil, "", err
	}

	return v, saveDir, nil
}

func (l *VersionLogic) createRollback(tx *ent.Tx, err error) (*ent.Version, string, error) {
	if rerr := tx.Rollback(); rerr != nil {
		l.logger.Error("Failed to rollback transaction",
			zap.Error(err),
		)
		err = fmt.Errorf("%w: %v", err, rerr)
	}
	return nil, "", err
}

func (l *VersionLogic) createRollbackRemoveSaveDir(tx *ent.Tx, err error, saveDir string) (*ent.Version, string, error) {
	rmerr := os.RemoveAll(saveDir)
	if rmerr != nil {
		l.logger.Error("Failed to remove storage directory",
			zap.Error(rmerr),
		)
		err = fmt.Errorf("%w: %v", err, rmerr)
	}
	return l.createRollback(tx, err)
}

type ListVersionParam struct {
	ResourceID int
	Offset     int
	Limit      int
}

func (l *VersionLogic) List(ctx context.Context, param ListVersionParam) (int, []*ent.Version, error) {
	query := l.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(param.ResourceID)))

	count, err := query.Count(ctx)
	if err != nil {
		return 0, nil, err
	}

	versions, err := query.
		Offset(param.Offset).
		Limit(param.Limit).
		Order(ent.Desc("number")).
		All(ctx)
	if err != nil {
		return 0, nil, err
	}

	return count, versions, nil
}

type GetVersionByNameParam struct {
	ResourceID int
	Name       string
}

func (l *VersionLogic) GetByName(ctx context.Context, param GetVersionByNameParam) (*ent.Version, error) {
	return l.db.Version.Query().
		Where(version.HasResourceWith(resource.ID(param.ResourceID)), version.Name(param.Name)).
		First(ctx)
}

func (l *VersionLogic) Delete(ctx context.Context, id int) error {
	return l.db.Version.
		DeleteOneID(id).
		Exec(ctx)
}
