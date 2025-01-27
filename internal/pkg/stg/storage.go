package stg

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

type Storage struct {
	RootDir string
}

func New() *Storage {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get current working directory, %v", err)
	}
	return &Storage{
		RootDir: filepath.Join(cwd, "storage"),
	}
}

func (s *Storage) ResourcePath(resID string, verID int) string {
	return filepath.Join(s.RootDir, resID, strconv.Itoa(verID), "resource.zip")
}

func (s *Storage) VersionDir(resID string, verID int) string {
	return filepath.Join(s.RootDir, resID, strconv.Itoa(verID))
}

func (s *Storage) PatchDir(resID string, targetVerID int) string {
	return filepath.Join(s.RootDir, resID, strconv.Itoa(targetVerID), "patch")
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
	return filepath.Join(s.RootDir, resID, strconv.Itoa(targetVerID), "patch", fmt.Sprintf("%d.zip", currentVerID))
}
