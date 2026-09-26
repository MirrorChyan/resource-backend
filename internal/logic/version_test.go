package logic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/MirrorChyan/resource-backend/internal/cache"
	"github.com/MirrorChyan/resource-backend/internal/ent"
	"github.com/MirrorChyan/resource-backend/internal/ent/storage"
	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archiver"
	"github.com/MirrorChyan/resource-backend/internal/repo"
	"github.com/MirrorChyan/resource-backend/internal/tasks"
)

func newPatchTestLogic(t *testing.T) (*VersionLogic, *ent.Client) {
	t.Helper()
	r, client := newTestRepo(t)
	addr := miniredis.RunT(t).Addr()
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	queue := asynq.NewClient(asynq.RedisClientOpt{Addr: addr})
	t.Cleanup(func() {
		_ = queue.Close()
		_ = rdb.Close()
	})
	dir := t.TempDir()
	return &VersionLogic{
		logger:      zap.NewNop(),
		repo:        r,
		versionRepo: repo.NewVersion(r),
		rdb:         rdb,
		taskQueue:   &tasks.TaskQueue{Client: queue},
		cacheGroup: &cache.MultiCacheGroup{
			VersionNameIdCache:         cache.NewCache[string, int](-1),
			IncrementalUpdateInfoCache: cache.NewCache[string, *model.IncrementalUpdateInfo](time.Hour),
		},
		storageLogic: &StorageLogic{
			logger:      zap.NewNop(),
			storageRepo: repo.NewStorage(r),
			RootDir:     filepath.Join(dir, "storage"),
			OSSDir:      filepath.Join(dir, "oss"),
		},
	}, client
}

type patchFixture struct {
	l          *VersionLogic
	client     *ent.Client
	param      model.PatchTaskExecuteParam
	patchDir   string
	localPatch string
	ossPatch   string
}

// newPatchFixture stores the package of a target version built from files and returns
// the task parameters to patch it from a current version whose files hash as current.
func newPatchFixture(t *testing.T, fileType types.FileType, files map[string][]byte, current map[string]string) patchFixture {
	t.Helper()
	ctx := context.Background()
	l, client := newPatchTestLogic(t)

	res := client.Resource.Create().SetID("res").SetName("res").SetDescription("").SaveX(ctx)
	cur := client.Version.Create().SetResource(res).SetName("v1").SetNumber(1).SaveX(ctx)
	tgt := client.Version.Create().SetResource(res).SetName("v2").SetNumber(2).SaveX(ctx)

	src := t.TempDir()
	hashes := make(map[string]string, len(files))
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(src, name), content, 0o644))
		sum := sha256.Sum256(content)
		hashes[name] = hex.EncodeToString(sum[:])
	}

	suffix := types.GetFileSuffix(fileType)
	pkg := l.storageLogic.BuildVersionResourceStoragePath(res.ID, tgt.ID, "", "", "resource"+suffix)
	require.NoError(t, os.MkdirAll(filepath.Dir(pkg), 0o755))
	switch fileType {
	case types.Zip:
		require.NoError(t, archiver.CompressToZip(src, pkg))
	case types.Tgz:
		require.NoError(t, archiver.CompressToTarGz(src, pkg))
	}
	stat, err := os.Stat(pkg)
	require.NoError(t, err)

	patchDir := l.storageLogic.BuildVersionPatchStorageDirPath(res.ID, tgt.ID, "", "")
	localPatch := filepath.Join(patchDir, strconv.Itoa(cur.ID)+suffix)
	return patchFixture{
		l:      l,
		client: client,
		param: model.PatchTaskExecuteParam{
			ResourceId:           res.ID,
			TargetOriginPackage:  pkg,
			TargetVersionId:      tgt.ID,
			CurrentVersionId:     cur.ID,
			TargetFileType:       string(fileType),
			TargetFileSize:       stat.Size(),
			CurrentFileType:      string(fileType),
			TargetStorageHashes:  hashes,
			CurrentStorageHashes: current,
		},
		patchDir:   patchDir,
		localPatch: localPatch,
		ossPatch:   filepath.Join(l.storageLogic.OSSDir, l.cleanRootStoragePath(localPatch)),
	}
}

func (f patchFixture) incrementalStorages(t *testing.T) []*ent.Storage {
	t.Helper()
	return f.client.Storage.Query().
		Where(storage.UpdateTypeEQ(storage.UpdateTypeIncremental)).
		AllX(context.Background())
}

func (f patchFixture) skipFlagged(t *testing.T) bool {
	t.Helper()
	p := f.param
	n, err := f.l.rdb.Exists(context.Background(),
		misc.PatchSkipKey(p.TargetVersionId, p.CurrentVersionId, p.OS, p.Arch),
	).Result()
	require.NoError(t, err)
	return n > 0
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

// every file of the target changed, so a patch would be as large as the full package
func allChangedFiles(t *testing.T) (map[string][]byte, map[string]string) {
	return map[string][]byte{
			"a.bin": randomBytes(t, 64<<10),
			"b.bin": randomBytes(t, 64<<10),
		}, map[string]string{
			"a.bin": "old-a",
			"b.bin": "old-b",
		}
}

func TestPatchSkippedWithoutCurrentFileHashes(t *testing.T) {
	f := newPatchFixture(t, types.Zip, map[string][]byte{"a.txt": []byte("a")}, nil)

	require.NoError(t, f.l.doCreateIncrementalUpdatePackage(context.Background(), f.param))

	require.NoDirExists(t, f.patchDir)
	require.NoFileExists(t, f.ossPatch)
	require.Empty(t, f.incrementalStorages(t))
	require.True(t, f.skipFlagged(t))
}

func TestPatchSkippedWhenEstimatedTooLarge(t *testing.T) {
	files, current := allChangedFiles(t)
	f := newPatchFixture(t, types.Zip, files, current)

	require.NoError(t, f.l.doCreateIncrementalUpdatePackage(context.Background(), f.param))

	// the zip estimate rejects it before anything is built
	require.NoDirExists(t, f.patchDir)
	require.NoFileExists(t, f.ossPatch)
	require.Empty(t, f.incrementalStorages(t))
	require.True(t, f.skipFlagged(t))
}

func TestPatchDiscardedWhenTooLarge(t *testing.T) {
	files, current := allChangedFiles(t)
	f := newPatchFixture(t, types.Tgz, files, current)

	require.NoError(t, f.l.doCreateIncrementalUpdatePackage(context.Background(), f.param))

	// a tgz cannot be estimated, so the built patch is measured and dropped
	require.NoFileExists(t, f.localPatch)
	require.NoFileExists(t, f.ossPatch)
	require.Empty(t, f.incrementalStorages(t))
	require.True(t, f.skipFlagged(t))
}

func TestPatchKeptWhenSmall(t *testing.T) {
	for _, fileType := range []types.FileType{types.Zip, types.Tgz} {
		t.Run(string(fileType), func(t *testing.T) {
			big := randomBytes(t, 64<<10)
			sum := sha256.Sum256(big)
			f := newPatchFixture(t, fileType,
				map[string][]byte{"big.bin": big, "small.txt": []byte("v2")},
				map[string]string{"big.bin": hex.EncodeToString(sum[:]), "small.txt": "old"},
			)

			require.NoError(t, f.l.doCreateIncrementalUpdatePackage(context.Background(), f.param))

			require.NoFileExists(t, f.localPatch)
			require.FileExists(t, f.ossPatch)
			stat, err := os.Stat(f.ossPatch)
			require.NoError(t, err)
			require.Less(t, stat.Size(), f.param.TargetFileSize/2)

			rows := f.incrementalStorages(t)
			require.Len(t, rows, 1)
			require.Equal(t, f.ossPatch, rows[0].PackagePath)
			require.Equal(t, stat.Size(), rows[0].FileSize)
			require.False(t, f.skipFlagged(t))
		})
	}
}

func TestUpdateRequestHonorsSkipFlag(t *testing.T) {
	for _, flagged := range []bool{false, true} {
		t.Run("flagged="+strconv.FormatBool(flagged), func(t *testing.T) {
			ctx := context.Background()
			f := newPatchFixture(t, types.Zip, map[string][]byte{"a.txt": []byte("a")}, nil)
			p := f.param
			if flagged {
				require.NoError(t, f.l.rdb.Set(ctx,
					misc.PatchSkipKey(p.TargetVersionId, p.CurrentVersionId, p.OS, p.Arch),
					misc.ProcessFlag, time.Hour,
				).Err())
			}

			result, err := f.l.doProcessUpdateRequest(ctx, model.UpdateRequestParam{
				ResourceId:         p.ResourceId,
				CurrentVersionName: "v1",
				TargetVersionInfo: &model.LatestVersionInfo{
					ResourceUpdateType: types.UpdateIncremental,
					VersionId:          p.TargetVersionId,
					PackagePath:        sql.NullString{String: p.TargetOriginPackage, Valid: true},
					FileSize:           p.TargetFileSize,
				},
			})
			require.NoError(t, err)
			require.Equal(t, types.UpdateFull.String(), result.UpdateType)

			// a flagged pair is served the full package without enqueuing the diff task again
			generated, err := f.l.rdb.Keys(ctx, misc.GenerateTagKey+":*").Result()
			require.NoError(t, err)
			if flagged {
				require.Empty(t, generated)
			} else {
				require.Len(t, generated, 1)
			}
		})
	}
}

func TestPatchTooLarge(t *testing.T) {
	require.False(t, patchTooLarge(79, 100))
	require.True(t, patchTooLarge(80, 100))
	require.True(t, patchTooLarge(120, 100))
	// unknown full size never rejects a patch
	require.False(t, patchTooLarge(120, 0))
}
