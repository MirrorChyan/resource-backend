package fileops

import (
	"github.com/MirrorChyan/resource-backend/internal/pkg/bufpool"
	"io"
	"os"

	"go.uber.org/zap"
)

func CopyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(source)

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func(destFile *os.File) {
		err := destFile.Close()
		if err != nil {
			zap.L().Error("Failed to close file",
				zap.String("file", destFile.Name()),
				zap.Error(err),
			)
		}
	}(dest)

	buf := bufpool.GetBuffer()
	defer bufpool.PutBuffer(buf)
	_, err = io.CopyBuffer(dest, source, buf)
	return err
}

func MoveFile(src, dst string) error {
	err := CopyFile(src, dst)
	if err != nil {
		return err
	}

	go func(src string) {
		_ = os.Remove(src)
	}(src)

	return nil
}
