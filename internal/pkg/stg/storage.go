package stg

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Storage struct {
	rootDir string
}

func New(cwd string) *Storage {
	rootDir := filepath.Join(cwd, "storage")
	return &Storage{
		rootDir: rootDir,
	}
}

func (s *Storage) ResourcePath(resID string, verID int) string {
	return filepath.Join(s.rootDir, resID, strconv.Itoa(verID), "resource.zip")
}

func (s *Storage) VersionDir(resID string, verID int) string {
	return filepath.Join(s.rootDir, resID, strconv.Itoa(verID))
}

func (s *Storage) PatchDir(resID string, targetVerID int) string {
	return filepath.Join(s.rootDir, resID, strconv.Itoa(targetVerID), "patch")
}

func (s *Storage) PatchPath(resID string, targetVerID, currentVerID int) string {
	path := s.buildPatchPath(resID, targetVerID, currentVerID)
	return path
}

func (s *Storage) PatchExists(resID string, targetVerID, currentVerID int) (bool, error) {
	path := s.buildPatchPath(resID, targetVerID, currentVerID)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func (s *Storage) buildPatchPath(resID string, targetVerID, currentVerID int) string {
	return filepath.Join(s.rootDir, resID, strconv.Itoa(targetVerID), "patch", fmt.Sprintf("%d.zip", currentVerID))
}
