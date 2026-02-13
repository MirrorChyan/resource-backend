package misc

import (
	"github.com/MirrorChyan/resource-backend/internal/pkg/errs"
	"github.com/gofiber/fiber/v2"
)

const ResourceKey = "rid"

const (
	ZipSuffix = ".zip"
	TgzSuffix = ".tar.gz"

	DefaultResourceName = "resource"

	DispensePrefix = "dispense"
)

const (
	ContentTypeZip   = "application/zip"
	ContentTypeTarGz = "application/x-gtar"
)

// used by diff
const (
	GenerateTagKey           = "generate"
	LoadStoreNewVersionKey   = "LoadStoreNewVersionTx"
	ProcessStoragePendingKey = "ProcessStoragePending"
)

const (
	ProcessFlag = "1"
)

// used by task
const (
	ProcessStorageTask = "storage"
	DiffTask           = "diff"
	PurgeTask          = "purge"
)

const (
	VersionPrefix = "ver"
)

const SniffLen = 4

var (
	ZipMagicHeader = []byte("PK\x03\x04")
	TgzMagicHeader = []byte("\x1F\x8B\x08")
)

var (
	StorageInfoNotFoundError = errs.NewUnchecked("storage info not found")

	NotAllowedFileTypeError = errs.NewUnchecked("not allowed file type")

	ResourceLimitError = errs.NewUnchecked("your cdkey has reached the most downloads today").WithHttpCode(fiber.StatusForbidden)
)

var (
	OsMap = map[string]string{
		// any
		"": "",

		// windows
		"windows": "windows",
		"win":     "windows",
		"win32":   "windows",

		// linux
		"linux": "linux",

		// darwin
		"darwin": "darwin",
		"macos":  "darwin",
		"mac":    "darwin",
		"osx":    "darwin",

		// android
		"android": "android",
	}

	ArchMap = map[string]string{
		// any
		"":    "",
		"any": "",

		// 386
		"386":    "386",
		"x86":    "386",
		"x86_32": "386",
		"i386":   "386",

		// amd64
		"amd64":   "amd64",
		"x64":     "amd64",
		"x86_64":  "amd64",
		"intel64": "amd64",

		// arm
		"arm": "arm",

		// arm64
		"arm64":   "arm64",
		"aarch64": "arm64",
	}

	ChannelMap = map[string]string{
		// stable
		"":       "stable",
		"stable": "stable",

		// beta
		"beta": "beta",

		// alpha
		"alpha": "alpha",
	}

	TotalChannel = []string{"stable", "beta", "alpha"}
	TotalOs      = []string{"", "windows", "linux", "darwin", "android"}
	TotalArch    = []string{"", "386", "arm64", "amd64", "arm"}
)
