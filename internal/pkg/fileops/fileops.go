package fileops

import (
	"github.com/gofiber/fiber/v2/log"
	"io"
	"os"
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
			log.Errorf("Failed to close file: %v %v", destFile.Name(), err)
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
