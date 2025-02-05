package patcher

import (
	"fmt"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
	"github.com/bytedance/sonic"
	"golang.org/x/sync/errgroup"
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

func Generate(patchName, resDir, targetDir string, changes []Change) (string, error) {
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

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	var files [][2]string

	for _, change := range changes {

		switch change.ChangeType {
		case Modified, Added:

			resPath := filepath.Join(resDir, change.Filename)
			tempPath := filepath.Join(root, change.Filename)

			tempFileDir := filepath.Dir(tempPath)
			_, err := os.Stat(tempFileDir)
			if err != nil {
				if !os.IsNotExist(err) {
					return "", fmt.Errorf("failed to stat temp file directory: %w", err)
				}
				if err := os.MkdirAll(tempFileDir, os.ModePerm); err != nil {
					return "", fmt.Errorf("failed to create temp file directory: %w", err)
				}
			}
			files = append(files, [2]string{resPath, tempPath})
		case Deleted:
			// do nothing
		case Unchanged:
			// do nothing
		default:
			return "", fmt.Errorf("unknown change type: %d", change.ChangeType)
		}
	}

	var (
		wg   = errgroup.Group{}
		flag = atomic.Bool{}
	)

	flag.Store(false)
	wg.SetLimit(runtime.NumCPU() * 10)
	for i := range files {
		if flag.Load() {
			break
		}
		wg.Go(func() error {
			if flag.Load() {
				return nil
			}
			src, dst := files[i][0], files[i][1]
			if err := fileops.CopyFile(src, dst); err != nil {
				flag.Store(true)
				return fmt.Errorf("failed to copy file: %w", err)
			}
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return "", err
	}

	changesJSONPath := filepath.Join(root, "changes.json")
	changesFile, err := os.Create(changesJSONPath)
	if err != nil {
		return "", fmt.Errorf("failed to create changes.json file: %w", err)
	}
	defer func(f *os.File) {
		_ = changesFile.Close()

	}(changesFile)

	changesMap := groupChangesByType(changes)
	jsonData, err := sonic.Marshal(changesMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal changes to JSON: %w", err)
	}

	if err := os.WriteFile(changesJSONPath, jsonData, 0644); err != nil {
		return "", fmt.Errorf("failed to write JSON to file: %w", err)
	}

	archiveName := fmt.Sprintf("%s.zip", patchName)
	archivePath := filepath.Join(targetDir, archiveName)
	err = archive.CompressToZip(root, archivePath)
	if err != nil {
		return "", err
	}

	return archiveName, nil
}
