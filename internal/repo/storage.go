package repo

import (
	"context"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
)

type Storage struct {
	db *ent.Client
}

func NewStorage(db *ent.Client) *Storage {
	return &Storage{
		db: db,
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
		Select(storage.FieldPackagePath).
		Where(
			storage.HasVersionWith(version.ID(verID)),
			storage.HasOldVersionWith(version.ID(oldVerID)),
			storage.UpdateTypeEQ(storage.UpdateTypeIncremental),
			storage.Os(os),
			storage.Arch(arch),
		).
		Only(ctx)
}

func (r *Storage) GetOldFullUpdateStorages(ctx context.Context, resID string, channel version.Channel, latestVerID int) ([]*ent.Storage, error) {
	return r.db.Storage.Query().
		Where(
			storage.HasVersionWith(
				version.HasResourceWith(resource.ID(resID)),
				version.ChannelEQ(channel),
				version.IDNEQ(latestVerID),
			),
			storage.UpdateTypeEQ(storage.UpdateTypeFull),
		).
		All(ctx)
}

func (r *Storage) ClearOldFullUpdateStorages(ctx context.Context, resID string, channel version.Channel, latestVerID int) error {
	err := r.db.Storage.Update().
		Where(
			storage.HasVersionWith(
				version.HasResourceWith(resource.ID(resID)),
				version.ChannelEQ(channel),
				version.IDNEQ(latestVerID),
			),
			storage.UpdateTypeEQ(storage.UpdateTypeFull),
		).
		ClearPackagePath().
		ClearResourcePath().
		Exec(ctx)
	return err
}

func (r *Storage) GetOldIncrementalUpdateStorages(ctx context.Context, resID string, channel version.Channel, latestVerID int) ([]*ent.Storage, error) {
	return r.db.Storage.Query().
		Where(
			storage.HasVersionWith(
				version.HasResourceWith(resource.ID(resID)),
				version.ChannelEQ(channel),
				version.IDNEQ(latestVerID),
			),
			storage.UpdateTypeEQ(storage.UpdateTypeIncremental),
		).
		All(ctx)
}

func (r *Storage) DeleteOldIncrementalUpdateStorages(ctx context.Context, resID string, channel version.Channel, latestVerID int) error {
	_, err := r.db.Storage.Delete().
		Where(
			storage.HasVersionWith(
				version.HasResourceWith(resource.ID(resID)),
				version.ChannelEQ(channel),
				version.IDNEQ(latestVerID),
			),
			storage.UpdateTypeEQ(storage.UpdateTypeIncremental),
		).
		Exec(ctx)
	return err
}

func (r *Storage) SetPackageHashSHA256(ctx context.Context, storageID int, packageSHA256 string) error {
	return r.db.Storage.UpdateOneID(storageID).
		SetPackageHashSha256(packageSHA256).
		Exec(ctx)
}
