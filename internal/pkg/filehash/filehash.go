package filehash

import (
	"encoding/hex"
	"github.com/MirrorChyan/resource-backend/internal/pkg"
	"github.com/minio/sha256-simd"
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

	h := sha256.New()
	buf := pkg.GetBuffer()
	defer pkg.PutBuffer(buf)
	if _, err := io.CopyBuffer(h, file, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
