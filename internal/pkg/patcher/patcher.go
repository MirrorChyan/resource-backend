package patcher

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/model"
	"github.com/MirrorChyan/resource-backend/internal/model/types"
	"github.com/MirrorChyan/resource-backend/internal/pkg/bufpool"
	"github.com/bytedance/sonic"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/MirrorChyan/resource-backend/internal/pkg/archiver"
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

func getChangesInfo(changes []Change) map[string][]string {
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

	buf := bufpool.GetBuffer()
	defer bufpool.PutBuffer(buf)
	_, err = io.CopyBuffer(dst, src, buf)

	return err
}

func extractTgzFile(origin string, pending map[string]string) error {
	file, err := os.Open(origin)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func(gzr *gzip.Reader) {
		_ = gzr.Close()
	}(gzr)

	reader := tar.NewReader(gzr)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg {

			if dest, ok := pending[header.Name]; ok {
				out, err := os.OpenFile(dest, os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
				if err != nil {
					return err
				}

				buf := bufpool.GetBuffer()
				_, err = io.CopyBuffer(out, reader, buf)

				bufpool.PutBuffer(buf)
				_ = out.Close()
			}

		}
	}

	return nil
}

func extractZipFile(origin string, pending map[string]string) error {
	var fileList = make([]transferInfo, 0, len(pending))
	reader, err := zip.OpenReader(origin)

	if err != nil {
		return err
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

	return wg.Wait()
}

func appendChangesRecord(root string, changes []Change) error {
	path := filepath.Join(root, "changes.json")
	data := getChangesInfo(changes)

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create changes.json file: %w", err)
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	buf, err := sonic.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal changes to JSON: %w", err)
	}

	if err := os.WriteFile(path, buf, 0644); err != nil {
		return fmt.Errorf("failed to write JSON to file: %w", err)
	}
	return nil
}

func GenerateV2(info model.PatchInfoTuple, changes []Change) error {

	var (
		origin = info.SrcPackage
		dest   = info.DestPackage
	)

	// create temp root dir
	root, err := os.MkdirTemp(os.TempDir(), "process-temp")
	if err != nil {
		return fmt.Errorf("failed to create temp root directory: %w", err)
	}
	// remove temp root dir
	defer func(p string) {
		go func(p string) {
			_ = os.RemoveAll(p)
		}(p)
	}(root)

	// inner file -> process full path
	var pending = make(map[string]string)

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
					return fmt.Errorf("failed to stat temp file directory: %w", err)
				}
				if err := os.MkdirAll(dir, os.ModePerm); err != nil {
					return fmt.Errorf("failed to create temp file directory: %w", err)
				}
			}
			pending[change.Filename] = tmp
		case Deleted:
			// do nothing
		case Unchanged:
			// do nothing
		default:
			return fmt.Errorf("unknown change type: %d", change.ChangeType)
		}
	}

	switch info.SrcFileType {
	case string(types.Zip):
		err := extractTgzFile(origin, pending)
		if err != nil {
			return fmt.Errorf("failed to extract zip file: %w", err)
		}
	case string(types.Tgz):
		err := extractZipFile(origin, pending)
		if err != nil {
			return fmt.Errorf("failed to extract tgz file: %w", err)
		}
	}

	err = appendChangesRecord(root, changes)
	if err != nil {
		return err
	}

	switch info.DestFileType {
	case string(types.Zip):
		if err = archiver.CompressToZip(root, dest); err != nil {
			return err
		}
	case string(types.Tgz):
		if err = archiver.CompressToTarGz(root, dest); err != nil {
			return err
		}
	}

	return nil
}
