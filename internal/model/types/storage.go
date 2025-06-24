package types

import "github.com/MirrorChyan/resource-backend/internal/logic/misc"

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

func GetFileSuffix(t FileType) string {
	switch t {
	case Zip:
		return misc.ZipSuffix
	case Tgz:
		return misc.TgzSuffix
	}
	return ""
}
