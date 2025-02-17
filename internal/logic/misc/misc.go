package misc

import (
	"errors"
	"sync"
	"sync/atomic"
)

const (
	ResourceKey     = "rid"
	RegionHeaderKey = "X-Region"
)

const (
	FullUpdateType        = "full"
	IncrementalUpdateType = "incremental"

	ResourcePrefix = "res"

	ZipSuffix         = ".zip"
	TarGzSuffix       = ".tar.gz"
	SpecificSeparator = "$#@"

	DispensePrefix = "dispense"
)

var (
	StorageInfoNotFound = errors.New("storage info not found")

	ResourceLimitError = errors.New("your cdkey has reached the most downloads today")
)

type RemoteError string

func (r RemoteError) Error() string {
	return string(r)
}

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
		"": "",

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
	AllChannel = []string{"stable", "beta", "alpha"}
)

var LIT = &sync.Map{}

func CompareIfAbsent(m *sync.Map, key string) *atomic.Int32 {
	value, ok := m.Load(key)
	if ok {
		return value.(*atomic.Int32)
	}

	r, _ := m.LoadOrStore(key, &atomic.Int32{})
	return r.(*atomic.Int32)
}
