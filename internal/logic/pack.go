package logic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	. "github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"github.com/MirrorChyan/resource-backend/internal/pkg/gitpack"
	"go.uber.org/zap"
)

// A pack resource serves a git branch: each version is the snapshot of a commit, named
// after its committer time, and the latest version is the one stored last. Uploading a
// stored version again publishes it again, which is how the branch moves back to an
// older commit; the version must then be the same commit.

// DoProcessPackStorage imports the snapshot uploaded for a version of a pack resource.
// Errors wrapping gitpack.ErrInvalid reject the upload for good.
func (l *VersionLogic) DoProcessPackStorage(ctx context.Context,
	resourceId string, versionId int,
	versionName, channel, system, arch, source string,
) error {
	snapshot, err := gitpack.Import(ctx, source, "")
	if err != nil {
		return err
	}
	defer func() {
		if err := snapshot.Close(); err != nil {
			l.logger.Warn("failed to remove snapshot repository", zap.Error(err))
		}
	}()

	if name := snapshot.VersionName(); name != versionName {
		return fmt.Errorf("%w: commit %s makes version %s, not %s", gitpack.ErrInvalid, snapshot.Commit, name, versionName)
	}

	stored, err := l.storageLogic.GetFullUpdateStorage(ctx, versionId, system, arch)
	switch {
	case err == nil:
		err = l.doRepublishPackVersion(ctx, stored, snapshot, resourceId, versionId, system, arch)
	case ent.IsNotFound(err):
		err = l.doStorePackVersion(ctx, snapshot, resourceId, versionId, system, arch)
		if err == nil {
			go l.doWebhookNotify(resourceId, versionName, channel, system, arch)
		}
	}
	if err != nil {
		return err
	}

	l.logger.Info("pack version stored",
		zap.String("resource id", resourceId),
		zap.String("version name", versionName),
		zap.String("commit", snapshot.Commit),
	)
	l.cacheGroup.FullUpdateStorageCache.Delete(l.cacheGroup.GetCacheKey(strconv.Itoa(versionId), system, arch))
	l.doPostCreateResources(resourceId)
	return nil
}

func (l *VersionLogic) doStorePackVersion(ctx context.Context, snapshot *gitpack.Snapshot,
	resourceId string, versionId int, system, arch string,
) error {
	path, hash, size, err := l.doWritePackArchive(snapshot, resourceId, versionId, system, arch)
	if err != nil {
		return err
	}
	_, err = l.storageLogic.CreatePackStorage(ctx, versionId, system, arch, path, hash, size, snapshot.Objects, snapshot.Commit)
	if err != nil {
		l.doRemoveFile(path)
		return err
	}
	return nil
}

func (l *VersionLogic) doRepublishPackVersion(ctx context.Context, stored *ent.Storage, snapshot *gitpack.Snapshot,
	resourceId string, versionId int, system, arch string,
) error {
	if stored.PackCommit != snapshot.Commit {
		// two commits of the same second share a name
		return fmt.Errorf("%w: the version is commit %q already, not %s", gitpack.ErrInvalid, stored.PackCommit, snapshot.Commit)
	}

	if stored.PackagePath != "" {
		if _, err := os.Stat(stored.PackagePath); err == nil {
			l.logger.Info("publish pack version again",
				zap.String("resource id", resourceId),
				zap.Int("version id", versionId),
			)
			return l.storageLogic.TouchStorage(ctx, stored.ID)
		}
	}

	// the purge removed the package, and the incremental packages to the version with it
	path, hash, size, err := l.doWritePackArchive(snapshot, resourceId, versionId, system, arch)
	if err != nil {
		return err
	}
	if err := l.storageLogic.RestorePackage(ctx, stored.ID, path, hash, size); err != nil {
		l.doRemoveFile(path)
		return err
	}
	from, err := l.storageLogic.DeletePurgedIncrementalStorages(ctx, versionId, system, arch)
	if err != nil {
		return err
	}
	// let the pairs be built again: their cached storages and queued tags outlive the packages
	for _, current := range from {
		l.cacheGroup.IncrementalUpdateInfoCache.Delete(
			l.cacheGroup.GetCacheKey(strconv.Itoa(versionId), strconv.Itoa(current), system, arch),
		)
		l.rdb.Del(ctx, misc.PatchGenerateTagKey(resourceId, versionId, current, system, arch))
	}
	l.logger.Info("restore purged pack version",
		zap.String("resource id", resourceId),
		zap.Int("version id", versionId),
		zap.Int("dropped incremental packages", len(from)),
	)
	return nil
}

// doWritePackArchive writes the package of the snapshot to OSS. The name carries the
// content hash: a package written again differs in bytes, and must not take the URL
// a CDN may still have the old one under.
func (l *VersionLogic) doWritePackArchive(snapshot *gitpack.Snapshot,
	resourceId string, versionId int, system, arch string,
) (path, hash string, size int64, err error) {
	tmp, err := os.MkdirTemp("", "pack-")
	if err != nil {
		return "", "", 0, err
	}
	defer l.doRemoveDir(tmp)

	local := filepath.Join(tmp, misc.PackSnapshotName+misc.ZipSuffix)
	if err := snapshot.WriteArchive(local); err != nil {
		return "", "", 0, err
	}
	hash, err = filehash.Calculate(local)
	if err != nil {
		return "", "", 0, err
	}
	stat, err := os.Stat(local)
	if err != nil {
		return "", "", 0, err
	}

	dir := l.storageLogic.BuildVersionStorageDirPath(resourceId, versionId, system, arch)
	name := misc.PackSnapshotName + "-" + hash[:16] + misc.ZipSuffix
	path = filepath.Join(l.storageLogic.OSSDir, l.cleanRootStoragePath(dir), name)
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return "", "", 0, err
	}
	if err := fileops.CopyFile(local, path); err != nil {
		l.logger.Error("failed to copy pack package to oss",
			zap.String("destination", path),
			zap.Error(err),
		)
		return "", "", 0, err
	}
	return path, hash, stat.Size(), nil
}

// doCreatePackIncrementalPackage builds the incremental package of a pack resource: the
// objects of the target's snapshot that the current version does not have.
func (l *VersionLogic) doCreatePackIncrementalPackage(ctx context.Context, param PatchTaskExecuteParam, currentCommit string) error {
	skip := l.patchSkipper(ctx, param)
	if len(param.CurrentStorageHashes) == 0 || currentCommit == "" {
		skip("the current version has no snapshot")
		return nil
	}
	if param.TargetOriginPackage == "" {
		return errors.New("the package of the target version is gone")
	}

	tmp, err := os.MkdirTemp("", "patch-")
	if err != nil {
		return fmt.Errorf("failed to create patch temp directory: %w", err)
	}
	defer l.doRemoveDir(tmp)

	dest := filepath.Join(tmp, strconv.Itoa(param.CurrentVersionId)+misc.ZipSuffix)
	if err := gitpack.Diff(ctx, param.TargetOriginPackage, param.CurrentStorageHashes, currentCommit, dest, tmp); err != nil {
		l.logger.Error("failed to build pack incremental package",
			zap.String("resource id", param.ResourceId),
			zap.Int("target version id", param.TargetVersionId),
			zap.Int("current version id", param.CurrentVersionId),
			zap.Error(err),
		)
		return err
	}
	return l.doStoreIncrementalPackage(ctx, param, dest, misc.ZipSuffix, skip)
}

func (l *VersionLogic) doRemoveFile(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		l.logger.Warn("failed to remove file", zap.String("path", path), zap.Error(err))
	}
}

func (l *VersionLogic) doRemoveDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		l.logger.Warn("failed to remove directory", zap.String("path", path), zap.Error(err))
	}
}
