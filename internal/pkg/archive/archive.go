package archive

import (
	"github.com/MirrorChyan/resource-backend/internal/pkg"

	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// UnpackZip unpacks a zip archive to the specified destination directory.
func UnpackZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func(r *zip.ReadCloser) {
		_ = r.Close()
	}(r)

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		defer func(f *os.File) {
			_ = f.Close()
		}(outFile)

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func(f io.ReadCloser) {
			_ = f.Close()
		}(rc)

		if _, err = io.Copy(outFile, rc); err != nil {
			return err
		}
	}
	return nil
}

// UnpackTarGz unpacks a tar.gz archive to the specified destination directory.
func UnpackTarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func(gzr *gzip.Reader) {
		_ = gzr.Close()
	}(gzr)

	tarReader := tar.NewReader(gzr)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		fpath := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fpath, os.ModePerm); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
				return err
			}

			outFile, err := os.Create(fpath)
			if err != nil {
				return err
			}
			defer func(outFile *os.File) {
				_ = outFile.Close()
			}(outFile)

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return err
			}
		}
	}
	return nil
}

// CompressToZip creates a ZIP archive from the specified source directory.
func CompressToZip(srcDir, destZip string) error {
	zipFile, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		if err := f.Close(); err != nil {
			zap.L().Error("Failed to close zip file",
				zap.Error(err),
			)
		}
	}(zipFile)

	writer := zip.NewWriter(zipFile)
	defer func(w *zip.Writer) {
		if err := w.Close(); err != nil {
			zap.L().Error("Failed to close zip writer",
				zap.Error(err),
			)
		}
	}(writer)

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func(file *os.File) {
			err := file.Close()
			if err != nil {
				zap.L().Error("Failed to close file",
					zap.String("file", file.Name()),
					zap.Error(err),
				)
			}
		}(file)

		zipFileWriter, err := writer.Create(relPath)
		if err != nil {
			return err
		}

		buf := pkg.GetBuffer()
		defer pkg.PutBuffer(buf)
		_, err = io.CopyBuffer(zipFileWriter, file, buf)
		return err
	})
}

// CompressToTarGz creates a TAR.GZ archive from the specified source directory.
func CompressToTarGz(srcDir, destTarGz string) error {
	tarGzFile, err := os.Create(destTarGz)
	if err != nil {
		return err
	}
	defer func(tarGzFile *os.File) {
		_ = tarGzFile.Close()
	}(tarGzFile)

	gzipWriter := gzip.NewWriter(tarGzFile)
	defer func(gzipWriter *gzip.Writer) {
		_ = gzipWriter.Close()
	}(gzipWriter)

	tarWriter := tar.NewWriter(gzipWriter)
	defer func(tarWriter *tar.Writer) {
		_ = tarWriter.Close()
	}(tarWriter)

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func(file *os.File) {
			_ = file.Close()
		}(file)

		header, err := tar.FileInfoHeader(info, relPath)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		_, err = io.Copy(tarWriter, file)
		return err
	})
}
