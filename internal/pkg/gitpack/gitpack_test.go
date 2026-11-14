package gitpack

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

// gitIn runs git in dir with fixed identities and dates; date is the committer date.
func gitIn(t *testing.T, dir string, stdin []byte, date string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(string(gitRaw(t, dir, stdin, date, args...)))
}

func gitRaw(t *testing.T, dir string, stdin []byte, date string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.autocrlf=false", "-c", "init.defaultBranch=master"}, args...)...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if date == "" {
		date = "2026-11-01T10:00:00+08:00"
	}
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.com", "GIT_COMMITTER_DATE="+date,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "git %v: %s", args, stderr.String())
	return stdout.Bytes()
}

type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	requireGit(t)
	dir := t.TempDir()
	gitIn(t, dir, nil, "", "init", "-q")
	return &repo{t: t, dir: dir}
}

func (r *repo) commit(date, message string, files map[string]string) string {
	for name, content := range files {
		path := filepath.Join(r.dir, name)
		require.NoError(r.t, os.MkdirAll(filepath.Dir(path), 0o755))
		if content == "" {
			require.NoError(r.t, os.Remove(path))
			continue
		}
		require.NoError(r.t, os.WriteFile(path, []byte(content), 0o644))
	}
	gitIn(r.t, r.dir, nil, date, "add", "-A")
	gitIn(r.t, r.dir, nil, date, "commit", "-q", "-m", message)
	return gitIn(r.t, r.dir, nil, "", "rev-parse", "HEAD")
}

// pack writes a pack of the object list, like CI does with rev-list | pack-objects.
func (r *repo) pack(objects string) string {
	data := gitRaw(r.t, r.dir, []byte(objects), "", "pack-objects", "-q", "--stdout")
	path := filepath.Join(r.t.TempDir(), "upload.pack")
	require.NoError(r.t, os.WriteFile(path, data, 0o644))
	return path
}

func (r *repo) snapshot(commit string) string {
	return r.pack(gitIn(r.t, r.dir, nil, "", "rev-list", "--objects", "--no-walk", commit) + "\n")
}

func importSnapshot(t *testing.T, pack string) *Snapshot {
	t.Helper()
	s, err := Import(context.Background(), pack, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestImport(t *testing.T) {
	r := newRepo(t)
	r.commit("2026-10-31T01:22:49+08:00", "c1", map[string]string{"a.txt": "1", "d/b.txt": "x"})
	c2 := r.commit("2026-10-31T01:30:00+08:00", "c2", map[string]string{"a.txt": "2", "d/e/c.txt": "y"})

	s := importSnapshot(t, r.snapshot(c2))
	require.Equal(t, c2, s.Commit)
	// the committer time in UTC, not in the zone the commit records
	require.Equal(t, "20261030173000", s.VersionName())
	require.Equal(t, map[string]string{
		RootTreeKey: gitIn(t, r.dir, nil, "", "rev-parse", c2+"^{tree}"),
		"a.txt":     gitIn(t, r.dir, nil, "", "rev-parse", c2+":a.txt"),
		"d":         gitIn(t, r.dir, nil, "", "rev-parse", c2+":d"),
		"d/b.txt":   gitIn(t, r.dir, nil, "", "rev-parse", c2+":d/b.txt"),
		"d/e":       gitIn(t, r.dir, nil, "", "rev-parse", c2+":d/e"),
		"d/e/c.txt": gitIn(t, r.dir, nil, "", "rev-parse", c2+":d/e/c.txt"),
	}, s.Objects)

	archive := filepath.Join(t.TempDir(), "full.zip")
	require.NoError(t, s.WriteArchive(archive))
	meta, err := ReadMeta(archive)
	require.NoError(t, err)
	require.Equal(t, Meta{Commit: c2}, meta)

	// the temporary repository goes away
	require.NoError(t, s.Close())
	_, err = os.Stat(s.dir)
	require.True(t, os.IsNotExist(err))
}

func TestImportRejects(t *testing.T) {
	r := newRepo(t)
	r.commit("", "c1", map[string]string{"a.txt": "1"})
	c2 := r.commit("", "c2", map[string]string{"a.txt": "2", "b.txt": "b"})
	blob := gitIn(t, r.dir, nil, "", "rev-parse", c2+":b.txt")

	var withoutBlob []string
	for _, line := range strings.Split(gitIn(t, r.dir, nil, "", "rev-list", "--objects", "--no-walk", c2), "\n") {
		if !strings.HasPrefix(line, blob) {
			withoutBlob = append(withoutBlob, line)
		}
	}

	evilTree := gitIn(t, r.dir, []byte("100644 blob "+blob+"\t.git\n"), "", "mktree")
	evil := gitIn(t, r.dir, []byte("evil"), "", "commit-tree", evilTree)

	garbage := filepath.Join(t.TempDir(), "garbage.pack")
	require.NoError(t, os.WriteFile(garbage, []byte("PACK not really"), 0o644))

	for name, pack := range map[string]string{
		"history":      r.pack(gitIn(t, r.dir, nil, "", "rev-list", "--objects", "HEAD") + "\n"),
		"missing blob": r.pack(strings.Join(withoutBlob, "\n") + "\n"),
		"dotgit entry": r.snapshot(evil),
		"garbage":      garbage,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Import(context.Background(), pack, t.TempDir())
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestDiff(t *testing.T) {
	r := newRepo(t)
	big := strings.Repeat("unchanged content\n", 2000)
	c1 := r.commit("", "c1", map[string]string{"a.txt": "1", "big.txt": big, "gone.txt": "bye", "d/b.txt": "x"})
	r.commit("", "c2", map[string]string{"a.txt": "2"})
	c3 := r.commit("", "c3", map[string]string{"a.txt": "3", "gone.txt": "", "d/c.txt": "new"})

	base := importSnapshot(t, r.snapshot(c1))
	target := importSnapshot(t, r.snapshot(c3))
	full := filepath.Join(t.TempDir(), "full.zip")
	require.NoError(t, target.WriteArchive(full))

	patch := filepath.Join(t.TempDir(), "patch.zip")
	require.NoError(t, Diff(context.Background(), full, base.Objects, c1, patch, t.TempDir()))
	meta, err := ReadMeta(patch)
	require.NoError(t, err)
	require.Equal(t, Meta{Commit: c3, Base: c1}, meta)

	fullInfo, err := os.Stat(full)
	require.NoError(t, err)
	patchInfo, err := os.Stat(patch)
	require.NoError(t, err)
	require.Less(t, patchInfo.Size(), fullInfo.Size())

	// a client at c1, the way a snapshot leaves it: c1 is a shallow boundary
	client := t.TempDir()
	gitIn(t, client, nil, "", "init", "-q")
	applyArchive(t, client, mustArchive(t, base))
	gitIn(t, client, nil, "", "reset", "-q", "--hard", c1)

	applyArchive(t, client, patch)
	gitIn(t, client, nil, "", "reset", "-q", "--hard", c3)
	require.Equal(t, gitIn(t, r.dir, nil, "", "rev-parse", c3+"^{tree}"), gitIn(t, client, nil, "", "write-tree"))
	gitIn(t, client, nil, "", "fsck", "--connectivity-only", "--no-progress")

	// the patch alone is not enough: it relies on the objects of c1
	empty := t.TempDir()
	gitIn(t, empty, nil, "", "init", "-q")
	extracted := t.TempDir()
	_, err = extractArchive(patch, extracted)
	require.NoError(t, err)
	packs, _ := filepath.Glob(filepath.Join(extracted, "*.pack"))
	data, err := os.ReadFile(packs[0])
	require.NoError(t, err)
	gitIn(t, empty, data, "", "index-pack", "--stdin")
	_, err = run(context.Background(), empty, nil, "rev-list", "--objects", "--no-walk", c3)
	require.Error(t, err)
}

func mustArchive(t *testing.T, s *Snapshot) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.zip")
	require.NoError(t, s.WriteArchive(path))
	return path
}

// applyArchive does what a client does with a package: index the pack, check the tree is
// complete and mark the commit as a shallow boundary.
func applyArchive(t *testing.T, repoDir, archive string) {
	t.Helper()
	dir := t.TempDir()
	meta, err := extractArchive(archive, dir)
	require.NoError(t, err)
	packs, err := filepath.Glob(filepath.Join(dir, "*.pack"))
	require.NoError(t, err)
	require.Len(t, packs, 1)
	data, err := os.ReadFile(packs[0])
	require.NoError(t, err)
	gitIn(t, repoDir, data, "", "index-pack", "--stdin")
	gitIn(t, repoDir, nil, "", "rev-list", "--objects", "--no-walk", meta.Commit)
	shallow := filepath.Join(repoDir, ".git", "shallow")
	f, err := os.OpenFile(shallow, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(meta.Commit + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestErrInvalidIsWrapped(t *testing.T) {
	require.True(t, errors.Is(invalid(errors.New("x")), ErrInvalid))
}
