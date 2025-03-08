package patcher

import (
	"archive/zip"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/pkg"
	"github.com/bytedance/sonic"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
)

type ChangeType int

const (
	Unchanged ChangeType = iota
	Modified
	Deleted
	Added
)

type Change struct {
	Filename   string     `json:"filename"`
	ChangeType ChangeType `json:"change_type"`
}

func groupChangesByType(changes []Change) map[string][]string {
	changesMap := make(map[string][]string)

	for _, change := range changes {
		switch change.ChangeType {
		case Modified:
			changesMap["modified"] = append(changesMap["modified"], change.Filename)
		case Deleted:
			changesMap["deleted"] = append(changesMap["deleted"], change.Filename)
		case Added:
			changesMap["added"] = append(changesMap["added"], change.Filename)
		case Unchanged:
			// changesMap["unchanged"] = append(changesMap["unchanged"], change.Filename)
		default:
			// todo
		}
	}

	return changesMap
}

func CalculateDiff(newVersionFileHashes, oldVersionFileHashes map[string]string) ([]Change, error) {
	var changes []Change

	for file, newHash := range newVersionFileHashes {
		if oldHash, exists := oldVersionFileHashes[file]; !exists {
			changes = append(changes, Change{Filename: file, ChangeType: Added})
		} else if oldHash != newHash {
			changes = append(changes, Change{Filename: file, ChangeType: Modified})
		} else {
			changes = append(changes, Change{Filename: file, ChangeType: Unchanged})
		}
	}

	for file := range oldVersionFileHashes {
		if _, exists := newVersionFileHashes[file]; !exists {
			changes = append(changes, Change{Filename: file, ChangeType: Deleted})
		}
	}

	return changes, nil
}

type transferInfo struct {
	src *zip.File
	dst string
}

func (t transferInfo) transfer() error {
	src, err := t.src.Open()
	if err != nil {
		return err
	}
	defer func(src io.ReadCloser) {
		_ = src.Close()
	}(src)

	dst, err := os.Create(t.dst)
	if err != nil {
		return err
	}

	defer func(dst *os.File) {
		_ = dst.Close()
	}(dst)

	buf := pkg.GetBuffer()
	defer pkg.PutBuffer(buf)
	_, err = io.CopyBuffer(dst, src, buf)

	return err
}

func GenerateV2(patchName, origin, dest string, changes []Change) (string, error) {
	// create temp root dir
	root, err := os.MkdirTemp(os.TempDir(), "process-temp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp root directory: %w", err)
	}
	// remove temp root dir
	defer func(p string) {
		go func(p string) {
			_ = os.RemoveAll(p)
		}(p)
	}(root)

	if err := os.MkdirAll(dest, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	var (
		pending  = make(map[string]string)
		fileList []transferInfo
	)

	for _, change := range changes {

		switch change.ChangeType {
		case Modified, Added:

			var (
				tmp = filepath.Join(root, change.Filename)
				dir = filepath.Dir(tmp)
			)
			_, err := os.Stat(dir)
			if err != nil {
				if !os.IsNotExist(err) {
					return "", fmt.Errorf("failed to stat temp file directory: %w", err)
				}
				if err := os.MkdirAll(dir, os.ModePerm); err != nil {
					return "", fmt.Errorf("failed to create temp file directory: %w", err)
				}
			}
			pending[change.Filename] = tmp
		case Deleted:
			// do nothing
		case Unchanged:
			// do nothing
		default:
			return "", fmt.Errorf("unknown change type: %d", change.ChangeType)
		}
	}

	reader, err := zip.OpenReader(origin)
	if err != nil {
		return "", err
	}
	defer func(r *zip.ReadCloser) {
		_ = r.Close()
	}(reader)

	for _, f := range reader.File {
		if val, ok := pending[f.Name]; ok {
			fileList = append(fileList, transferInfo{
				src: f,
				dst: val,
			})
		}
	}

	var (
		wg   = errgroup.Group{}
		flag = atomic.Bool{}
	)

	flag.Store(false)
	wg.SetLimit(runtime.NumCPU() * 10)
	for _, t := range fileList {
		if flag.Load() {
			break
		}
		wg.Go(func() error {
			if flag.Load() {
				return nil
			}
			if err = t.transfer(); err != nil {
				flag.Store(true)
				return fmt.Errorf("failed to transfer file: %w", err)
			}
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return "", err
	}

	var (
		p    = filepath.Join(root, "changes.json")
		data = groupChangesByType(changes)
	)

	f, err := os.Create(p)
	if err != nil {
		return "", fmt.Errorf("failed to create changes.json file: %w", err)
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	buf, err := sonic.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal changes to JSON: %w", err)
	}

	if err := os.WriteFile(p, buf, 0644); err != nil {
		return "", fmt.Errorf("failed to write JSON to file: %w", err)
	}

	var (
		archiveName = fmt.Sprintf("%s.zip", patchName)
		archivePath = filepath.Join(dest, archiveName)
	)

	if err = archive.CompressToZip(root, archivePath); err != nil {
		return "", err
	}

	return archiveName, nil
}
