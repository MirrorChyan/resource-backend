package filehash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

func Calculate(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func GetAll(targetDir string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			hash, err := Calculate(path)
			if err != nil {
				return err
			}
			relativePath, _ := filepath.Rel(targetDir, path)
			files[relativePath] = hash
		}
		return nil
	})
	return files, err
}
