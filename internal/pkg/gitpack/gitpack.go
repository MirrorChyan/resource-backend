// Package gitpack builds the packages of pack resources with the git command line.
//
// A snapshot is one commit with its whole tree and none of its history, as a non-thin pack.
// The package of a version is an archive of the snapshot, and an incremental package is an
// archive of the objects of the target snapshot that the client's version does not have.
// Clients put the pack into their repository and mark the commit as a shallow boundary.
package gitpack

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrInvalid marks a snapshot that can never be imported, so trying again is pointless.
var ErrInvalid = errors.New("invalid snapshot")

// RootTreeKey is the key of the root tree in Snapshot.Objects.
const RootTreeKey = "."

// VersionNameLayout formats the committer time, in UTC, into the name of the version:
// clients only have their HEAD and must reach the same name from it.
const VersionNameLayout = "20060102150405"

const metaName = "meta.json"

// Meta is meta.json in a package archive.
type Meta struct {
	Commit string `json:"commit"`
	// Base is the commit an incremental package applies to; empty for a snapshot.
	Base string `json:"base"`
}

// Snapshot is an imported snapshot, held in a temporary repository until Close.
type Snapshot struct {
	Commit     string
	CommitTime time.Time
	// Objects maps each path of the tree, directories included, and RootTreeKey to the
	// ID of its object: an incremental package leaves out the objects its base has.
	Objects map[string]string

	dir  string
	pack string
	idx  string
}

// VersionName is the name the version of the snapshot must have.
func (s *Snapshot) VersionName() string {
	return s.CommitTime.UTC().Format(VersionNameLayout)
}

// WriteArchive writes the package of the snapshot's version.
func (s *Snapshot) WriteArchive(dest string) error {
	return writeArchive(dest, s.pack, s.idx, Meta{Commit: s.Commit})
}

// Close removes the temporary repository.
func (s *Snapshot) Close() error {
	return os.RemoveAll(s.dir)
}

// Import checks an uploaded snapshot: an intact non-thin pack of exactly one commit whose
// tree is complete. Errors wrapping ErrInvalid reject the snapshot itself.
func Import(ctx context.Context, packFile, tmpRoot string) (_ *Snapshot, err error) {
	dir, err := os.MkdirTemp(tmpRoot, "gitpack-import-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()

	if _, err := run(ctx, dir, nil, "init", "-q", "--bare"); err != nil {
		return nil, err
	}

	f, err := os.Open(packFile)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	// --fsck-objects checks every object, rejecting e.g. a tree with a .git entry;
	// --strict would also demand the parent commits, which a snapshot leaves out
	out, err := run(ctx, dir, f, "index-pack", "--fsck-objects", "--stdin")
	if err != nil {
		return nil, invalid(err)
	}
	pack, idx, err := packFiles(filepath.Join(dir, "objects", "pack"), out)
	if err != nil {
		return nil, err
	}

	listing, err := run(ctx, dir, nil, "cat-file", "--batch-all-objects", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return nil, err
	}
	var commits []string
	for _, line := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		if oid, kind, ok := strings.Cut(line, " "); ok && kind == "commit" {
			commits = append(commits, oid)
		}
	}
	if len(commits) != 1 {
		return nil, invalid(fmt.Errorf("the pack holds %d commits, a snapshot holds one", len(commits)))
	}
	commit := commits[0]

	tree, committed, err := readCommit(ctx, dir, commit)
	if err != nil {
		return nil, err
	}
	// every tree and file of the commit must be in the pack
	if _, err := run(ctx, dir, nil, "rev-list", "--objects", "--no-walk", commit); err != nil {
		return nil, invalid(fmt.Errorf("the tree of %s is incomplete: %w", commit, err))
	}
	objects, err := treeObjects(ctx, dir, commit)
	if err != nil {
		return nil, err
	}
	objects[RootTreeKey] = tree

	return &Snapshot{
		Commit:     commit,
		CommitTime: committed,
		Objects:    objects,
		dir:        dir,
		pack:       pack,
		idx:        idx,
	}, nil
}

// Diff writes the incremental package from the base version to the snapshot in the
// target archive: the objects of the target whose IDs are not among base, the objects of
// the base version, as a non-thin pack. The client has the base commit checked out.
func Diff(ctx context.Context, targetArchive string, base map[string]string, baseCommit, dest, tmpRoot string) (err error) {
	dir, err := os.MkdirTemp(tmpRoot, "gitpack-diff-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	if _, err := run(ctx, dir, nil, "init", "-q", "--bare"); err != nil {
		return err
	}
	meta, err := extractArchive(targetArchive, filepath.Join(dir, "objects", "pack"))
	if err != nil {
		return err
	}

	have := make(map[string]struct{}, len(base))
	for _, oid := range base {
		have[oid] = struct{}{}
	}
	listing, err := run(ctx, dir, nil, "rev-list", "--objects", "--no-walk", meta.Commit)
	if err != nil {
		return err
	}
	var want bytes.Buffer
	for _, line := range strings.Split(string(listing), "\n") {
		oid, _, _ := strings.Cut(line, " ")
		if oid == "" {
			continue
		}
		if _, ok := have[oid]; ok {
			continue
		}
		// keep the path: pack-objects groups objects of one path when looking for deltas
		want.WriteString(line)
		want.WriteByte('\n')
	}

	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		return err
	}
	out, err := run(ctx, dir, &want, "pack-objects", "-q", filepath.Join(outDir, "pack"))
	if err != nil {
		return err
	}
	pack, idx, err := packFiles(outDir, out)
	if err != nil {
		return err
	}
	return writeArchive(dest, pack, idx, Meta{Commit: meta.Commit, Base: baseCommit})
}

// ReadMeta reads meta.json of a package archive.
func ReadMeta(archive string) (Meta, error) {
	var meta Meta
	r, err := zip.OpenReader(archive)
	if err != nil {
		return meta, err
	}
	defer func() {
		_ = r.Close()
	}()
	for _, f := range r.File {
		if f.Name == metaName {
			return readMeta(f)
		}
	}
	return meta, fmt.Errorf("%s has no %s", archive, metaName)
}

func readMeta(f *zip.File) (Meta, error) {
	var meta Meta
	rc, err := f.Open()
	if err != nil {
		return meta, err
	}
	defer func() {
		_ = rc.Close()
	}()
	err = json.NewDecoder(rc).Decode(&meta)
	return meta, err
}

func invalid(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalid, err)
}

// packFiles returns the pack and index named by the hash index-pack or pack-objects printed.
func packFiles(dir string, out []byte) (pack, idx string, err error) {
	hash := strings.TrimSpace(string(out))
	// index-pack prints "pack\t<hash>", pack-objects just the hash
	if _, h, ok := strings.Cut(hash, "\t"); ok {
		hash = h
	}
	if hash == "" {
		return "", "", errors.New("git did not name the written pack")
	}
	base := filepath.Join(dir, "pack-"+hash)
	pack, idx = base+".pack", base+".idx"
	for _, f := range []string{pack, idx} {
		if _, err := os.Stat(f); err != nil {
			return "", "", err
		}
	}
	return pack, idx, nil
}

// readCommit reads the header of the commit object itself: log and show would also load
// the parents, which a snapshot does not have.
func readCommit(ctx context.Context, dir, commit string) (tree string, committed time.Time, err error) {
	raw, err := run(ctx, dir, nil, "cat-file", "commit", commit)
	if err != nil {
		return "", time.Time{}, err
	}
	header, _, _ := strings.Cut(string(raw), "\n\n")
	for _, line := range strings.Split(header, "\n") {
		switch {
		case strings.HasPrefix(line, "tree "):
			tree = strings.TrimPrefix(line, "tree ")
		case strings.HasPrefix(line, "committer "):
			// committer <name> <<email>> <unix time> <offset>
			fields := strings.Fields(line)
			if len(fields) < 3 {
				return "", time.Time{}, invalid(fmt.Errorf("malformed committer of %s", commit))
			}
			sec, err := strconv.ParseInt(fields[len(fields)-2], 10, 64)
			if err != nil {
				return "", time.Time{}, invalid(fmt.Errorf("malformed committer time of %s", commit))
			}
			committed = time.Unix(sec, 0).UTC()
		}
	}
	if tree == "" || committed.IsZero() {
		return "", time.Time{}, invalid(fmt.Errorf("commit %s lacks a tree or committer", commit))
	}
	return tree, committed, nil
}

func treeObjects(ctx context.Context, dir, commit string) (map[string]string, error) {
	raw, err := run(ctx, dir, nil, "ls-tree", "-r", "-t", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	objects := make(map[string]string)
	for _, entry := range bytes.Split(raw, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		// <mode> SP <type> SP <oid> TAB <path>
		info, path, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("unexpected ls-tree entry %q", entry)
		}
		fields := strings.Fields(string(info))
		if len(fields) != 3 {
			return nil, fmt.Errorf("unexpected ls-tree entry %q", entry)
		}
		// a submodule's commit lives in another repository, the snapshot never has it
		if fields[1] != "blob" && fields[1] != "tree" {
			continue
		}
		objects[string(path)] = fields[2]
	}
	return objects, nil
}

func writeArchive(dest, pack, idx string, meta Meta) (err error) {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	w := zip.NewWriter(f)
	for _, src := range []string{pack, idx} {
		if err := addFile(w, src); err != nil {
			return err
		}
	}
	m, err := w.Create(metaName)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(m).Encode(meta); err != nil {
		return err
	}
	return w.Close()
}

func addFile(w *zip.Writer, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()
	// pack data is compressed already
	out, err := w.CreateHeader(&zip.FileHeader{Name: filepath.Base(src), Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return err
}

// extractArchive puts the pack and index of an archive into dir and returns its meta.
func extractArchive(archive, dir string) (Meta, error) {
	var meta Meta
	r, err := zip.OpenReader(archive)
	if err != nil {
		return meta, err
	}
	defer func() {
		_ = r.Close()
	}()
	found := false
	for _, f := range r.File {
		name := filepath.Base(f.Name)
		switch {
		case f.Name == metaName:
			if meta, err = readMeta(f); err != nil {
				return meta, err
			}
			found = true
		case strings.HasSuffix(name, ".pack") || strings.HasSuffix(name, ".idx"):
			if err := extractFile(f, filepath.Join(dir, name)); err != nil {
				return meta, err
			}
		}
	}
	if !found || meta.Commit == "" {
		return meta, fmt.Errorf("%s has no commit in its %s", archive, metaName)
	}
	return meta, nil
}

func extractFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func run(ctx context.Context, dir string, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
