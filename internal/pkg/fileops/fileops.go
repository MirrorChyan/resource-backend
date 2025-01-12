package fileops

import (
	"io"
	"os"

	"go.uber.org/zap"
)

func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(sourceFile *os.File) {
		err := sourceFile.Close()
		if err != nil {

		}
	}(sourceFile)

	destFile, err := os.Create(dst)
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
	}(destFile)

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func MoveFile(src, dst string) error {
	err := CopyFile(src, dst)
	if err != nil {
		return err
	}

	err = os.Remove(src)
	return err
}
