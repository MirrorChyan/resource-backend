package types

import (
	"strings"

	"github.com/MirrorChyan/resource-backend/internal/logic/misc"
)

type Update string

const (
	UpdateFull        Update = "full"
	UpdateIncremental Update = "incremental"
	// UpdatePack serves git commits: snapshots and the objects between two of them
	UpdatePack Update = "pack"
)

func (u Update) String() string {
	return string(u)
}

type FileType string

const (
	Tgz FileType = "tgz"
	Zip FileType = "zip"
	// Pack is an uploaded git pack, the snapshot of a commit
	Pack FileType = "pack"
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

// IsPackFile reports whether the file is named as a git pack.
func IsPackFile(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), misc.PackSuffix)
}

func GetFileSuffix(t FileType) string {
	switch t {
	case Zip:
		return misc.ZipSuffix
	case Tgz:
		return misc.TgzSuffix
	case Pack:
		return misc.PackSuffix
	}
	return ""
}
