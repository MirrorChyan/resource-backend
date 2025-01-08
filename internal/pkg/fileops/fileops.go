package fileops

import (
	"io"
	"os"
)

func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

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
