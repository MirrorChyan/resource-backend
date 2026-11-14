package repo

import (
	"context"
	"time"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/resource"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/ent/version"
	"github.com/MirrorChyan/resource-backend/internal/model"
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

// HasPlatform includes purged packages and all channels: either still counts as a prior upload.
func (r *Storage) HasPlatform(ctx context.Context, rid, os, arch string) (bool, error) {
	return r.db.Storage.Query().Where(
		storage.HasVersionWith(version.HasResourceWith(resource.ID(rid))),
		storage.Os(os),
		storage.Arch(arch),
	).Exist(ctx)
}

// ListPlatforms returns the platforms the resource has full packages stored for.
func (r *Storage) ListPlatforms(ctx context.Context, rid string) ([]model.Platform, error) {
	var platforms []model.Platform
	err := r.db.Storage.Query().
		Where(
			storage.HasVersionWith(version.HasResourceWith(resource.ID(rid))),
			storage.UpdateTypeEQ(storage.UpdateTypeFull),
		).
		GroupBy(storage.FieldOs, storage.FieldArch).
		Scan(ctx, &platforms)
	return platforms, err
}

func (r *Storage) UpdateStoragePackageHash(ctx context.Context, id int, hash string) error {
	return r.db.Storage.UpdateOneID(id).SetPackageHashSha256(hash).Exec(ctx)
}

// CreatePackStorage records the package of a version of a pack resource: an archive of the
// snapshot, the IDs of its objects and its commit.
func (r *Storage) CreatePackStorage(ctx context.Context, verID int,
	os, arch, path, hash string,
	size int64,
	objects map[string]string,
	commit string,
) (*ent.Storage, error) {
	return r.db.Storage.Create().
		SetUpdateType(storage.UpdateTypeFull).
		SetOs(os).
		SetArch(arch).
		SetPackagePath(path).
		SetPackageHashSha256(hash).
		SetFileType(string(types.Zip)).
		SetFileSize(size).
		SetFileHashes(objects).
		SetPackCommit(commit).
		SetVersionID(verID).
		Save(ctx)
}

// TouchStorage makes the storage the newest of its platform: the latest version and the
// purge both go by the time a storage was created.
func (r *Storage) TouchStorage(ctx context.Context, id int) error {
	return r.db.Storage.UpdateOneID(id).SetCreatedAt(time.Now()).Exec(ctx)
}

// RestorePackage gives a purged storage a package again and makes it the newest of its platform.
func (r *Storage) RestorePackage(ctx context.Context, id int, path, hash string, size int64) error {
	return r.db.Storage.UpdateOneID(id).
		SetPackagePath(path).
		SetPackageHashSha256(hash).
		SetFileSize(size).
		SetCreatedAt(time.Now()).
		Exec(ctx)
}

// DeletePurgedIncrementalStorages drops the incremental storages to the version whose
// packages the purge removed, and returns the versions they started from.
func (r *Storage) DeletePurgedIncrementalStorages(ctx context.Context, verID int, os, arch string) ([]int, error) {
	rows, err := r.db.Storage.Query().
		Where(
			storage.HasVersionWith(version.ID(verID)),
			storage.UpdateTypeEQ(storage.UpdateTypeIncremental),
			storage.Os(os),
			storage.Arch(arch),
			storage.PackagePathIsNil(),
		).
		WithOldVersion().
		All(ctx)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	ids := make([]int, 0, len(rows))
	from := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		if row.Edges.OldVersion != nil {
			from = append(from, row.Edges.OldVersion.ID)
		}
	}
	if _, err := r.db.Storage.Delete().Where(storage.IDIn(ids...)).Exec(ctx); err != nil {
		return nil, err
	}
	return from, nil
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

// PurgeStorageInfo clears package_path of the storages (full and incremental)
// that share the version and platform of storageId. Other platforms of the same
// version are left untouched: their files are not removed by this purge, and
// clearing them would orphan those files and hide still-retained packages.
func (r *Storage) PurgeStorageInfo(ctx context.Context, storageId int) error {
	val, err := r.db.Storage.Query().
		Where(storage.IDEQ(storageId)).
		First(ctx)
	if err != nil {
		return err
	}
	return r.db.Storage.Update().
		Where(
			storage.VersionStorages(val.VersionStorages),
			storage.Os(val.Os),
			storage.Arch(val.Arch),
		).
		ClearPackagePath().
		Exec(ctx)
}
