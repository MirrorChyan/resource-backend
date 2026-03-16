package patcher

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/archiver"
	"github.com/MirrorChyan/resource-backend/internal/pkg/filehash"
	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- helpers ----------

// writeFile creates a file with content under dir, creating intermediate directories as needed.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), os.ModePerm))
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
}

// buildZip creates a zip archive from srcDir and returns the archive path.
func buildZip(t *testing.T, srcDir, name string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), name)
	require.NoError(t, archiver.CompressToZip(srcDir, dest))
	return dest
}

// buildTgz creates a tgz archive from srcDir and returns the archive path.
func buildTgz(t *testing.T, srcDir, name string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), name)
	require.NoError(t, archiver.CompressToTarGz(srcDir, dest))
	return dest
}

// toSlashSlice normalizes all paths in a slice to forward slashes for cross-platform assertions.
// Returns nil if input is nil (preserves nil vs empty distinction).
func toSlashSlice(s []string) []string {
	if s == nil {
		return nil
	}
	r := make([]string, len(s))
	for i, v := range s {
		r[i] = filepath.ToSlash(v)
	}
	return r
}

// normalizeHashes converts all hash map keys from OS-native separators to forward slashes.
func normalizeHashes(h map[string]string) map[string]string {
	n := make(map[string]string, len(h))
	for k, v := range h {
		n[filepath.ToSlash(k)] = v
	}
	return n
}

// readChangesJSON reads and unmarshals changes.json from a directory,
// normalizing all path values to forward slashes.
func readChangesJSON(t *testing.T, dir string) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "changes.json"))
	require.NoError(t, err)
	var result map[string][]string
	require.NoError(t, sonic.Unmarshal(data, &result))
	// normalize all values to forward slashes for cross-platform comparison
	for k, v := range result {
		result[k] = toSlashSlice(v)
	}
	return result
}

// sortedSlice returns a sorted copy of the input slice.
func sortedSlice(s []string) []string {
	if s == nil {
		return nil
	}
	c := make([]string, len(s))
	copy(c, s)
	sort.Strings(c)
	return c
}

// classifyChanges groups a []Change into maps by ChangeType for easier assertion.
// Filenames are normalized to forward slashes.
func classifyChanges(changes []Change) map[ChangeType][]string {
	m := make(map[ChangeType][]string)
	for _, c := range changes {
		m[c.ChangeType] = append(m[c.ChangeType], filepath.ToSlash(c.Filename))
	}
	for k := range m {
		sort.Strings(m[k])
	}
	return m
}

// ---------- extractDirs ----------

func TestExtractDirs(t *testing.T) {
	tests := []struct {
		name     string
		hashes   map[string]string
		expected []string
	}{
		{
			name:     "root level files only",
			hashes:   map[string]string{"a.txt": "h1", "b.txt": "h2"},
			expected: nil,
		},
		{
			name:     "single level directory",
			hashes:   map[string]string{"dir/a.txt": "h1"},
			expected: []string{"dir"},
		},
		{
			name:     "nested directories",
			hashes:   map[string]string{"a/b/c/file.txt": "h1"},
			expected: []string{"a", "a/b", "a/b/c"},
		},
		{
			name: "multiple branches",
			hashes: map[string]string{
				"x/1.txt":   "h1",
				"y/z/2.txt": "h2",
			},
			expected: []string{"x", "y", "y/z"},
		},
		{
			name:     "empty hashes",
			hashes:   map[string]string{},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := extractDirs(tt.hashes)
			var got []string
			for d := range dirs {
				got = append(got, filepath.ToSlash(d))
			}
			sort.Strings(got)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// ---------- CalculateDirDiff ----------

func TestCalculateDirDiff(t *testing.T) {
	tests := []struct {
		name          string
		newHashes     map[string]string
		oldHashes     map[string]string
		expectAdded   []string
		expectDeleted []string
	}{
		{
			name:          "new directory added",
			newHashes:     map[string]string{"a/1.txt": "h1", "b/2.txt": "h2"},
			oldHashes:     map[string]string{"a/1.txt": "h1"},
			expectAdded:   []string{"b"},
			expectDeleted: nil,
		},
		{
			name:          "directory deleted",
			newHashes:     map[string]string{"a/1.txt": "h1"},
			oldHashes:     map[string]string{"a/1.txt": "h1", "b/2.txt": "h2"},
			expectAdded:   nil,
			expectDeleted: []string{"b"},
		},
		{
			name:          "nested directory added and another deleted",
			newHashes:     map[string]string{"a/1.txt": "h1", "c/d/3.txt": "h3"},
			oldHashes:     map[string]string{"a/1.txt": "h1", "b/2.txt": "h2"},
			expectAdded:   []string{"c", "c/d"},
			expectDeleted: []string{"b"},
		},
		{
			name:          "same directories no change",
			newHashes:     map[string]string{"a/1.txt": "h1_new", "a/2.txt": "h2"},
			oldHashes:     map[string]string{"a/1.txt": "h1_old", "a/3.txt": "h3"},
			expectAdded:   nil,
			expectDeleted: nil,
		},
		{
			name:          "root files only no dirs",
			newHashes:     map[string]string{"x.txt": "h1"},
			oldHashes:     map[string]string{"y.txt": "h2"},
			expectAdded:   nil,
			expectDeleted: nil,
		},
		{
			name:          "both empty",
			newHashes:     map[string]string{},
			oldHashes:     map[string]string{},
			expectAdded:   nil,
			expectDeleted: nil,
		},
		{
			name: "deep nesting added",
			newHashes: map[string]string{
				"a/1.txt":       "h1",
				"x/y/z/deep.go": "h2",
			},
			oldHashes:     map[string]string{"a/1.txt": "h1"},
			expectAdded:   []string{"x", "x/y", "x/y/z"},
			expectDeleted: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, deleted := CalculateDirDiff(tt.newHashes, tt.oldHashes)
			assert.Equal(t, sortedSlice(tt.expectAdded), sortedSlice(toSlashSlice(added)), "added dirs mismatch")
			assert.Equal(t, sortedSlice(tt.expectDeleted), sortedSlice(toSlashSlice(deleted)), "deleted dirs mismatch")
		})
	}
}

// ---------- CalculateDiff ----------

func TestCalculateDiff(t *testing.T) {
	newHashes := map[string]string{
		"a/1.txt":     "hash_a1_new",  // modified
		"a/2.txt":     "hash_a2_same", // unchanged
		"c/d/new.txt": "hash_new",     // added
		"root.txt":    "hash_root",    // added
	}
	oldHashes := map[string]string{
		"a/1.txt":   "hash_a1_old",  // modified
		"a/2.txt":   "hash_a2_same", // unchanged
		"b/old.txt": "hash_old",     // deleted
	}

	changes, err := CalculateDiff(newHashes, oldHashes)
	require.NoError(t, err)

	classified := classifyChanges(changes)

	assert.Equal(t, []string{"a/1.txt"}, classified[Modified])
	assert.Equal(t, []string{"a/2.txt"}, classified[Unchanged])
	assert.Equal(t, []string{"c/d/new.txt", "root.txt"}, classified[Added])
	assert.Equal(t, []string{"b/old.txt"}, classified[Deleted])
}

// ---------- getChangesInfo ----------

func TestGetChangesInfo(t *testing.T) {
	changes := []Change{
		{Filename: "a/1.txt", ChangeType: Modified},
		{Filename: "b/old.txt", ChangeType: Deleted},
		{Filename: "c/new.txt", ChangeType: Added},
		{Filename: "a/2.txt", ChangeType: Unchanged},
	}

	t.Run("with dir changes", func(t *testing.T) {
		info := getChangesInfo(changes, []string{"c"}, []string{"b"})

		assert.Equal(t, []string{"a/1.txt"}, info["modified"])
		assert.Equal(t, []string{"b/old.txt"}, info["deleted"])
		assert.Equal(t, []string{"c/new.txt"}, info["added"])
		assert.Equal(t, []string{"c"}, info["added_dir"])
		assert.Equal(t, []string{"b"}, info["deleted_dir"])
		_, hasUnchanged := info["unchanged"]
		assert.False(t, hasUnchanged, "unchanged should not be in output")
	})

	t.Run("no dir changes", func(t *testing.T) {
		info := getChangesInfo(changes, nil, nil)

		assert.Equal(t, []string{"a/1.txt"}, info["modified"])
		_, hasAddedDir := info["added_dir"]
		assert.False(t, hasAddedDir, "added_dir should not be present when empty")
		_, hasDeletedDir := info["deleted_dir"]
		assert.False(t, hasDeletedDir, "deleted_dir should not be present when empty")
	})
}

// ---------- Full pipeline: zip ----------

// TestFullPipelineZip tests the entire flow with zip archives:
// build v1 zip -> build v2 zip -> compute hashes -> diff -> dir diff -> GenerateV2 -> verify changes.json
func TestFullPipelineZip(t *testing.T) {
	// ── Build v1 directory structure ──
	// v1: a/1.txt, a/2.txt, b/3.txt, b/sub/4.txt
	v1Dir := t.TempDir()
	writeFile(t, v1Dir, "a/1.txt", "content_a1_v1")
	writeFile(t, v1Dir, "a/2.txt", "content_a2")
	writeFile(t, v1Dir, "b/3.txt", "content_b3")
	writeFile(t, v1Dir, "b/sub/4.txt", "content_b_sub_4")

	// ── Build v2 directory structure ──
	// v2: a/1.txt (modified), a/2.txt (unchanged), c/5.txt (new dir+file), c/d/6.txt (nested new)
	// b/ directory entirely removed
	v2Dir := t.TempDir()
	writeFile(t, v2Dir, "a/1.txt", "content_a1_v2_modified")
	writeFile(t, v2Dir, "a/2.txt", "content_a2")
	writeFile(t, v2Dir, "c/5.txt", "content_c5")
	writeFile(t, v2Dir, "c/d/6.txt", "content_c_d_6")

	// ── Create zip archives ──
	v1Zip := buildZip(t, v1Dir, "v1.zip")
	v2Zip := buildZip(t, v2Dir, "v2.zip")

	// ── Compute file hashes (same as production flow) ──
	v1Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v1Zip, v1Unpack))
	v1Hashes := normalizeHashes(must(filehash.GetAll(v1Unpack)))

	v2Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v2Zip, v2Unpack))
	v2Hashes := normalizeHashes(must(filehash.GetAll(v2Unpack)))

	// ── Verify hashes contain expected files ──
	assert.Contains(t, v1Hashes, "a/1.txt")
	assert.Contains(t, v1Hashes, "a/2.txt")
	assert.Contains(t, v1Hashes, "b/3.txt")
	assert.Contains(t, v1Hashes, "b/sub/4.txt")
	assert.Len(t, v1Hashes, 4)

	assert.Contains(t, v2Hashes, "a/1.txt")
	assert.Contains(t, v2Hashes, "a/2.txt")
	assert.Contains(t, v2Hashes, "c/5.txt")
	assert.Contains(t, v2Hashes, "c/d/6.txt")
	assert.Len(t, v2Hashes, 4)

	// ── CalculateDiff ──
	changes, err := CalculateDiff(v2Hashes, v1Hashes)
	require.NoError(t, err)

	classified := classifyChanges(changes)
	assert.Equal(t, []string{"a/1.txt"}, classified[Modified], "a/1.txt should be modified")
	assert.Equal(t, []string{"a/2.txt"}, classified[Unchanged], "a/2.txt should be unchanged")
	assert.Equal(t, []string{"c/5.txt", "c/d/6.txt"}, classified[Added], "new files should be added")
	assert.Equal(t, []string{"b/3.txt", "b/sub/4.txt"}, classified[Deleted], "old files should be deleted")

	// ── CalculateDirDiff ──
	addedDirs, deletedDirs := CalculateDirDiff(v2Hashes, v1Hashes)

	assert.Equal(t, []string{"c", "c/d"}, sortedSlice(toSlashSlice(addedDirs)), "new directories should be detected")
	assert.Equal(t, []string{"b", "b/sub"}, sortedSlice(toSlashSlice(deletedDirs)), "removed directories should be detected")

	// ── GenerateV2 (produce incremental zip package) ──
	patchDest := filepath.Join(t.TempDir(), "patch.zip")
	tuple := model.PatchInfoTuple{
		SrcPackage:  v2Zip,
		DestPackage: patchDest,
		FileType:    string(types.Zip),
	}
	require.NoError(t, GenerateV2(tuple, changes, addedDirs, deletedDirs))

	// ── Unpack the incremental package and verify ──
	patchUnpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(patchDest, patchUnpack))

	// Verify changes.json exists and has correct content
	cj := readChangesJSON(t, patchUnpack)

	assert.Equal(t, []string{"a/1.txt"}, cj["modified"], "changes.json modified")
	assert.Equal(t, sortedSlice([]string{"c/5.txt", "c/d/6.txt"}), sortedSlice(cj["added"]), "changes.json added")
	assert.Equal(t, sortedSlice([]string{"b/3.txt", "b/sub/4.txt"}), sortedSlice(cj["deleted"]), "changes.json deleted")
	assert.Equal(t, sortedSlice([]string{"c", "c/d"}), sortedSlice(cj["added_dir"]), "changes.json added_dir")
	assert.Equal(t, sortedSlice([]string{"b", "b/sub"}), sortedSlice(cj["deleted_dir"]), "changes.json deleted_dir")
	_, hasUnchanged := cj["unchanged"]
	assert.False(t, hasUnchanged, "unchanged should not appear in changes.json")

	// Verify that modified/added files are actually present in the patch
	assertFileExists(t, patchUnpack, "a/1.txt")
	assertFileExists(t, patchUnpack, "c/5.txt")
	assertFileExists(t, patchUnpack, "c/d/6.txt")

	// Verify the modified file has the new content
	content, err := os.ReadFile(filepath.Join(patchUnpack, "a/1.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content_a1_v2_modified", string(content))

	// Verify deleted files are NOT in the patch
	assertFileNotExists(t, patchUnpack, "b/3.txt")
	assertFileNotExists(t, patchUnpack, "b/sub/4.txt")

	// Verify added directory entries exist in the zip archive
	assertZipHasDirEntry(t, patchDest, "c/")
	assertZipHasDirEntry(t, patchDest, "c/d/")
}

// ---------- Full pipeline: tgz ----------

func TestFullPipelineTgz(t *testing.T) {
	// ── Build v1 ──
	v1Dir := t.TempDir()
	writeFile(t, v1Dir, "lib/core.so", "core_v1")
	writeFile(t, v1Dir, "lib/utils.so", "utils_same")
	writeFile(t, v1Dir, "config/app.toml", "config_v1")

	// ── Build v2 ──
	// lib/core.so modified, lib/utils.so unchanged
	// config/ removed, plugin/init.lua added
	v2Dir := t.TempDir()
	writeFile(t, v2Dir, "lib/core.so", "core_v2_updated")
	writeFile(t, v2Dir, "lib/utils.so", "utils_same")
	writeFile(t, v2Dir, "plugin/init.lua", "plugin_init")

	// ── Create tgz archives ──
	v1Tgz := buildTgz(t, v1Dir, "v1.tar.gz")
	v2Tgz := buildTgz(t, v2Dir, "v2.tar.gz")

	// ── Compute hashes ──
	v1Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackTarGz(v1Tgz, v1Unpack))
	v1Hashes := normalizeHashes(must(filehash.GetAll(v1Unpack)))

	v2Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackTarGz(v2Tgz, v2Unpack))
	v2Hashes := normalizeHashes(must(filehash.GetAll(v2Unpack)))

	// ── CalculateDiff ──
	changes, err := CalculateDiff(v2Hashes, v1Hashes)
	require.NoError(t, err)

	classified := classifyChanges(changes)
	assert.Equal(t, []string{"lib/core.so"}, classified[Modified])
	assert.Equal(t, []string{"lib/utils.so"}, classified[Unchanged])
	assert.Equal(t, []string{"plugin/init.lua"}, classified[Added])
	assert.Equal(t, []string{"config/app.toml"}, classified[Deleted])

	// ── CalculateDirDiff ──
	addedDirs, deletedDirs := CalculateDirDiff(v2Hashes, v1Hashes)
	assert.Equal(t, []string{"plugin"}, sortedSlice(toSlashSlice(addedDirs)))
	assert.Equal(t, []string{"config"}, sortedSlice(toSlashSlice(deletedDirs)))

	// ── GenerateV2 ──
	patchDest := filepath.Join(t.TempDir(), "patch.tar.gz")
	tuple := model.PatchInfoTuple{
		SrcPackage:  v2Tgz,
		DestPackage: patchDest,
		FileType:    string(types.Tgz),
	}
	require.NoError(t, GenerateV2(tuple, changes, addedDirs, deletedDirs))

	// ── Unpack and verify ──
	patchUnpack := t.TempDir()
	require.NoError(t, archiver.UnpackTarGz(patchDest, patchUnpack))

	cj := readChangesJSON(t, patchUnpack)
	assert.Equal(t, []string{"lib/core.so"}, cj["modified"])
	assert.Equal(t, []string{"plugin/init.lua"}, cj["added"])
	assert.Equal(t, []string{"config/app.toml"}, cj["deleted"])
	assert.Equal(t, []string{"plugin"}, sortedSlice(cj["added_dir"]))
	assert.Equal(t, []string{"config"}, sortedSlice(cj["deleted_dir"]))

	// Verify patched files
	assertFileExists(t, patchUnpack, "lib/core.so")
	assertFileExists(t, patchUnpack, "plugin/init.lua")
	content, err := os.ReadFile(filepath.Join(patchUnpack, "lib/core.so"))
	require.NoError(t, err)
	assert.Equal(t, "core_v2_updated", string(content))

	assertFileNotExists(t, patchUnpack, "config/app.toml")
}

// ---------- Edge cases ----------

func TestFullPipeline_NoChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/1.txt", "same")

	pkg := buildZip(t, dir, "v.zip")

	unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(pkg, unpack))
	hashes := normalizeHashes(must(filehash.GetAll(unpack)))

	changes, err := CalculateDiff(hashes, hashes)
	require.NoError(t, err)

	classified := classifyChanges(changes)
	assert.Len(t, classified[Modified], 0)
	assert.Len(t, classified[Added], 0)
	assert.Len(t, classified[Deleted], 0)
	assert.Equal(t, []string{"a/1.txt"}, classified[Unchanged])

	addedDirs, deletedDirs := CalculateDirDiff(hashes, hashes)
	assert.Nil(t, addedDirs)
	assert.Nil(t, deletedDirs)

	patchDest := filepath.Join(t.TempDir(), "patch.zip")
	tuple := model.PatchInfoTuple{
		SrcPackage:  pkg,
		DestPackage: patchDest,
		FileType:    string(types.Zip),
	}
	require.NoError(t, GenerateV2(tuple, changes, addedDirs, deletedDirs))

	patchUnpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(patchDest, patchUnpack))

	cj := readChangesJSON(t, patchUnpack)
	_, hasMod := cj["modified"]
	_, hasAdd := cj["added"]
	_, hasDel := cj["deleted"]
	_, hasAddDir := cj["added_dir"]
	_, hasDelDir := cj["deleted_dir"]
	assert.False(t, hasMod)
	assert.False(t, hasAdd)
	assert.False(t, hasDel)
	assert.False(t, hasAddDir)
	assert.False(t, hasDelDir)
}

func TestFullPipeline_AllFilesNew(t *testing.T) {
	v2Dir := t.TempDir()
	writeFile(t, v2Dir, "x/y/z/deep.txt", "deep_content")
	writeFile(t, v2Dir, "root.txt", "root_content")

	v2Zip := buildZip(t, v2Dir, "v2.zip")

	v2Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v2Zip, v2Unpack))
	v2Hashes := normalizeHashes(must(filehash.GetAll(v2Unpack)))

	oldHashes := map[string]string{} // empty = brand new

	changes, err := CalculateDiff(v2Hashes, oldHashes)
	require.NoError(t, err)

	classified := classifyChanges(changes)
	assert.Len(t, classified[Modified], 0)
	assert.Len(t, classified[Deleted], 0)
	assert.Len(t, classified[Unchanged], 0)
	assert.Equal(t, []string{"root.txt", "x/y/z/deep.txt"}, classified[Added])

	addedDirs, deletedDirs := CalculateDirDiff(v2Hashes, oldHashes)
	assert.Equal(t, []string{"x", "x/y", "x/y/z"}, sortedSlice(toSlashSlice(addedDirs)))
	assert.Nil(t, deletedDirs)
}

func TestFullPipeline_AllFilesDeleted(t *testing.T) {
	v1Dir := t.TempDir()
	writeFile(t, v1Dir, "a/b/1.txt", "content")

	v1Zip := buildZip(t, v1Dir, "v1.zip")

	v1Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v1Zip, v1Unpack))
	v1Hashes := normalizeHashes(must(filehash.GetAll(v1Unpack)))

	newHashes := map[string]string{} // everything removed

	changes, err := CalculateDiff(newHashes, v1Hashes)
	require.NoError(t, err)

	classified := classifyChanges(changes)
	assert.Len(t, classified[Modified], 0)
	assert.Len(t, classified[Added], 0)
	assert.Len(t, classified[Unchanged], 0)
	assert.Equal(t, []string{"a/b/1.txt"}, classified[Deleted])

	addedDirs, deletedDirs := CalculateDirDiff(newHashes, v1Hashes)
	assert.Nil(t, addedDirs)
	assert.Equal(t, []string{"a", "a/b"}, sortedSlice(toSlashSlice(deletedDirs)))
}

func TestFullPipeline_DirRename(t *testing.T) {
	// Simulate directory rename: old/ -> new/
	v1Dir := t.TempDir()
	writeFile(t, v1Dir, "old/file.txt", "content")

	v2Dir := t.TempDir()
	writeFile(t, v2Dir, "new/file.txt", "content") // same content, different dir

	v1Zip := buildZip(t, v1Dir, "v1.zip")
	v2Zip := buildZip(t, v2Dir, "v2.zip")

	v1Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v1Zip, v1Unpack))
	v1Hashes := normalizeHashes(must(filehash.GetAll(v1Unpack)))

	v2Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v2Zip, v2Unpack))
	v2Hashes := normalizeHashes(must(filehash.GetAll(v2Unpack)))

	changes, err := CalculateDiff(v2Hashes, v1Hashes)
	require.NoError(t, err)

	classified := classifyChanges(changes)
	assert.Equal(t, []string{"new/file.txt"}, classified[Added])
	assert.Equal(t, []string{"old/file.txt"}, classified[Deleted])

	addedDirs, deletedDirs := CalculateDirDiff(v2Hashes, v1Hashes)
	assert.Equal(t, []string{"new"}, sortedSlice(toSlashSlice(addedDirs)))
	assert.Equal(t, []string{"old"}, sortedSlice(toSlashSlice(deletedDirs)))

	// Verify full GenerateV2 pipeline
	patchDest := filepath.Join(t.TempDir(), "patch.zip")
	tuple := model.PatchInfoTuple{
		SrcPackage:  v2Zip,
		DestPackage: patchDest,
		FileType:    string(types.Zip),
	}
	require.NoError(t, GenerateV2(tuple, changes, addedDirs, deletedDirs))

	patchUnpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(patchDest, patchUnpack))

	cj := readChangesJSON(t, patchUnpack)
	assert.Equal(t, []string{"new/file.txt"}, cj["added"])
	assert.Equal(t, []string{"old/file.txt"}, cj["deleted"])
	assert.Equal(t, []string{"new"}, sortedSlice(cj["added_dir"]))
	assert.Equal(t, []string{"old"}, sortedSlice(cj["deleted_dir"]))
}

func TestFullPipeline_RootOnlyFiles(t *testing.T) {
	// No directories at all, only root-level files
	v1Dir := t.TempDir()
	writeFile(t, v1Dir, "old.txt", "old")

	v2Dir := t.TempDir()
	writeFile(t, v2Dir, "new.txt", "new")

	v1Zip := buildZip(t, v1Dir, "v1.zip")
	v2Zip := buildZip(t, v2Dir, "v2.zip")

	v1Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v1Zip, v1Unpack))
	v1Hashes := normalizeHashes(must(filehash.GetAll(v1Unpack)))

	v2Unpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(v2Zip, v2Unpack))
	v2Hashes := normalizeHashes(must(filehash.GetAll(v2Unpack)))

	addedDirs, deletedDirs := CalculateDirDiff(v2Hashes, v1Hashes)
	assert.Nil(t, addedDirs, "no dirs should be added with root-only files")
	assert.Nil(t, deletedDirs, "no dirs should be deleted with root-only files")

	changes, err := CalculateDiff(v2Hashes, v1Hashes)
	require.NoError(t, err)

	patchDest := filepath.Join(t.TempDir(), "patch.zip")
	tuple := model.PatchInfoTuple{
		SrcPackage:  v2Zip,
		DestPackage: patchDest,
		FileType:    string(types.Zip),
	}
	require.NoError(t, GenerateV2(tuple, changes, addedDirs, deletedDirs))

	patchUnpack := t.TempDir()
	require.NoError(t, archiver.UnpackZip(patchDest, patchUnpack))

	cj := readChangesJSON(t, patchUnpack)
	_, hasAddDir := cj["added_dir"]
	_, hasDelDir := cj["deleted_dir"]
	assert.False(t, hasAddDir)
	assert.False(t, hasDelDir)
}

// ---------- assertion helpers ----------

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func assertFileExists(t *testing.T, dir, rel string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	_, err := os.Stat(p)
	assert.NoError(t, err, "expected file to exist: %s", rel)
}

func assertFileNotExists(t *testing.T, dir, rel string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	_, err := os.Stat(p)
	assert.True(t, os.IsNotExist(err), "expected file to not exist: %s", rel)
}

func assertZipHasDirEntry(t *testing.T, zipPath, dirName string) {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer func() { _ = r.Close() }()

	found := false
	for _, f := range r.File {
		if f.Name == dirName {
			found = true
			break
		}
	}
	assert.True(t, found, "zip should contain directory entry: %s", dirName)
}
