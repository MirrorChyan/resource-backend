package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

type Storage struct {
	*Repo
}

func NewStorage(db *Repo) *Storage {
	return &Storage{
		Repo: db,
	}
}

func (r *Storage) CreateFullUpdateStorage(ctx context.Context, tx *ent.Tx, verID int, os, arch, fullUpdatePath, packageSHA256, resourcePath string, fileHashes map[string]string) (*ent.Storage, error) {
	return tx.Storage.Create().
		SetUpdateType(storage.UpdateTypeFull).
		SetOs(os).
		SetArch(arch).
		SetPackagePath(fullUpdatePath).
		SetPackageHashSha256(packageSHA256).
		SetResourcePath(resourcePath).
		SetFileHashes(fileHashes).
		SetVersionID(verID).
		Save(ctx)
}

func (r *Storage) CreateIncrementalUpdateStorage(ctx context.Context, tx *ent.Tx, verID, oldVerID int, os, arch, incrementalUpdatePath, packageSHA256 string) (*ent.Storage, error) {
	return tx.Storage.Create().
		SetUpdateType(storage.UpdateTypeIncremental).
		SetOs(os).
		SetArch(arch).
		SetPackagePath(incrementalUpdatePath).
		SetPackageHashSha256(packageSHA256).
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
	return r.db.Storage.UpdateOneID(storageId).
		SetPackagePath("").
		SetResourcePath("").
		Exec(ctx)
}
