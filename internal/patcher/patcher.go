package patcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MirrorChyan/resource-backend/internal/pkg/archive"
	"github.com/MirrorChyan/resource-backend/internal/pkg/fileops"
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
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	tempRootDir := filepath.Join(cwd, "temp")
	if err := os.MkdirAll(tempRootDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create temp root directory: %w", err)
	}
	tempDir, err := os.MkdirTemp(tempRootDir, "patch")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	for _, change := range changes {
		resPath := filepath.Join(resDir, change.Filename)
		tempPath := filepath.Join(tempDir, change.Filename)

		switch change.ChangeType {
		case Modified, Added:
			tempFileDir := filepath.Dir(tempPath)
			if err := os.MkdirAll(tempFileDir, os.ModePerm); err != nil {
				return "", fmt.Errorf("failed to create temp file directory: %w", err)
			}

			if err := fileops.CopyFile(resPath, tempPath); err != nil {
				return "", fmt.Errorf("failed to copy file: %w", err)
			}
		case Deleted:
			// do nothing
		case Unchanged:
			// do nothing
		default:
			return "", fmt.Errorf("unknown change type: %d", change.ChangeType)
		}
	}

	changesJSONPath := filepath.Join(tempDir, "changes.json")
	changesFile, err := os.Create(changesJSONPath)
	if err != nil {
		return "", fmt.Errorf("failed to create changes.json file: %w", err)
	}
	defer changesFile.Close()

	changesMap := groupChangesByType(changes)
	jsonData, err := json.MarshalIndent(changesMap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal changes to JSON: %w", err)
	}

	if err := os.WriteFile(changesJSONPath, jsonData, 0644); err != nil {
		return "", fmt.Errorf("failed to write JSON to file: %w", err)
	}

	archiveName := fmt.Sprintf("%s.zip", patchName)
	archivePath := filepath.Join(targetDir, archiveName)
	archive.CompressToZip(tempDir, archivePath)

	return archiveName, nil
}
