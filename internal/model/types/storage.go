package types

import (
	"strings"

	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
)

type Update string

const (
	UpdateFull        Update = "full"
	UpdateIncremental Update = "incremental"
)

func (u Update) String() string {
	return string(u)
}

type FileType string

const (
	Tgz FileType = "tgz"
	Zip FileType = "zip"
)

// IncrementalFileType returns only formats supported by both unpacking and patching.
func IncrementalFileType(filename string) FileType {
	filename = strings.ToLower(filename)
	switch {
	case strings.HasSuffix(filename, misc.ZipSuffix):
		return Zip
	case strings.HasSuffix(filename, misc.TgzSuffix):
		return Tgz
	default:
		return ""
	}
}

func GetFileSuffix(t FileType) string {
	switch t {
	case Zip:
		return misc.ZipSuffix
	case Tgz:
		return misc.TgzSuffix
	}
	return ""
}
