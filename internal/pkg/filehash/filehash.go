package filehash

import (
	"encoding/hex"
	"github.com/MirrorChyan/resource-backend/internal/pkg"
	"github.com/minio/sha256-simd"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
)

func Calculate(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(file)

	h := sha256.New()
	buf := pkg.GetBuffer()
	defer pkg.PutBuffer(buf)
	if _, err := io.CopyBuffer(h, file, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func GetAll(targetDir string) (map[string]string, error) {
	var (
		files = make(map[string]string)
		//    path,relativePath,hash
		tmp = make([][3]string, 0, 12)
	)
	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			tmp = append(tmp, [3]string{path, "", ""})
		}
		return nil
	})

	var (
		wg   = errgroup.Group{}
		flag = atomic.Bool{}
	)
	flag.Store(false)
	wg.SetLimit(runtime.NumCPU())
	for i := range tmp {
		if flag.Load() {
			break
		}
		wg.Go(func() error {
			if flag.Load() {
				return nil
			}
			path := tmp[i][0]
			_, e := os.Stat(path)
			if e != nil {
				return nil
			}
			hash, err := Calculate(path)
			if err != nil {
				flag.Store(true)
				return err
			}
			rel, err := filepath.Rel(targetDir, path)
			if err != nil {
				flag.Store(true)
				return err
			}
			tmp[i][1], tmp[i][2] = rel, hash
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, err
	}
	for i := range tmp {
		files[tmp[i][1]] = tmp[i][2]
	}
	return files, err
}
