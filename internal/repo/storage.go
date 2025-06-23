package repo

import (
	"context"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
)

type Storage struct {
	*Repo
}

func NewStorage(db *Repo) *Storage {
	return &Storage{
		Repo: db,
	}
}

func (r *Storage) CreateFullUpdateStorage(ctx context.Context, verID int,
	os, arch, path, hash string, fileType types.FileType,
	size int64,
	fileHashes map[string]string,
) (*ent.Storage, error) {

	return r.db.Storage.Create().
		SetUpdateType(storage.UpdateTypeFull).
		SetOs(os).
		SetArch(arch).
		SetPackagePath(path).
		SetPackageHashSha256(hash).
		SetFileType(string(fileType)).
		SetFileSize(size).
		SetFileHashes(fileHashes).
		SetVersionID(verID).
		Save(ctx)
}

func (r *Storage) CreateIncrementalUpdateStorage(ctx context.Context, tx *ent.Tx,
	verID, oldVerID int, filetype string, filesize int64,
	os, arch, updatePath, hashes string,
) (*ent.Storage, error) {
	return tx.Storage.Create().
		SetUpdateType(storage.UpdateTypeIncremental).
		SetOs(os).
		SetArch(arch).
		SetPackagePath(updatePath).
		SetFileSize(filesize).
		SetFileType(filetype).
		SetPackageHashSha256(hashes).
		SetVersionID(verID).
		SetOldVersionID(oldVerID).
		Save(ctx)
}

func (r *Storage) CheckStorageExist(ctx context.Context, verID int, os, arch string) (bool, error) {
	return r.db.Storage.Query().
		Where(
			storage.HasVersionWith(version.ID(verID)),
			storage.Os(os),
			storage.Arch(arch),
		).
		Exist(ctx)
}

func (r *Storage) GetFullUpdateStorage(ctx context.Context, verID int, os, arch string) (*ent.Storage, error) {
	return r.db.Storage.Query().
		Where(
			storage.HasVersionWith(version.ID(verID)),
			storage.UpdateTypeEQ(storage.UpdateTypeFull),
			storage.Os(os),
			storage.Arch(arch),
		).
		Only(ctx)
}

func (r *Storage) UpdateStoragePackageHash(ctx context.Context, id int, hash string) error {
	return r.db.Storage.UpdateOneID(id).SetPackageHashSha256(hash).Exec(ctx)
}

func (r *Storage) GetIncrementalUpdateStorage(ctx context.Context, verID, oldVerID int, os, arch string) (*ent.Storage, error) {
	return r.db.Storage.Query().
		Where(
			storage.HasVersionWith(version.ID(verID)),
			storage.HasOldVersionWith(version.ID(oldVerID)),
			storage.UpdateTypeEQ(storage.UpdateTypeIncremental),
			storage.Os(os),
			storage.Arch(arch),
		).
		Only(ctx)
}

func (r *Storage) PurgeStorageInfo(ctx context.Context, storageId int) error {
	val, err := r.db.Storage.Query().
		Where(storage.IDEQ(storageId)).
		First(ctx)
	if err != nil {
		return err
	}
	vid := val.VersionStorages
	err = r.db.Storage.Update().Where(storage.HasVersionWith(version.ID(vid))).
		ClearPackagePath().
		Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}
