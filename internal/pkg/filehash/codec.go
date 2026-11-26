package filehash

import (
	"fmt"

	"github.com/bytedance/sonic"
	"github.com/klauspost/compress/zstd"
)

// FormatZstdJSON is the storage_file_hashes.format value for zstd-compressed,
// key-sorted JSON (sonic.ConfigStd output).
const FormatZstdJSON uint8 = 1

// MaxDecoderMemory bounds the memory zstd may allocate when decompressing a
// single value, guarding against corrupted or crafted blobs.
const MaxDecoderMemory = 64 << 20

// EncodedHashes holds the compressed representation of a storage's file hashes.
type EncodedHashes struct {
	Format  uint8
	Data    []byte
	RawSize int
	Entries int
}

var (
	zstdDefaultEncoder *zstd.Encoder
	zstdDecoder        *zstd.Decoder
)

func init() {
	var err error
	zstdDefaultEncoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		panic(fmt.Errorf("init zstd default encoder: %w", err))
	}
	zstdDecoder, err = zstd.NewReader(nil, zstd.WithDecoderMaxMemory(MaxDecoderMemory))
	if err != nil {
		panic(fmt.Errorf("init zstd decoder: %w", err))
	}
}

// MarshalHashes serializes hashes with sonic.ConfigStd (deterministic sorted keys).
func MarshalHashes(hashes map[string]string) ([]byte, error) {
	raw, err := sonic.ConfigStd.Marshal(hashes)
	if err != nil {
		return nil, fmt.Errorf("marshal file hashes: %w", err)
	}
	return raw, nil
}

// EncodeHashes serializes hashes with sonic.ConfigStd (deterministic sorted keys)
// and compresses the result with zstd. Empty maps return (nil, nil).
func EncodeHashes(hashes map[string]string) (*EncodedHashes, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	raw, err := MarshalHashes(hashes)
	if err != nil {
		return nil, err
	}
	data := zstdDefaultEncoder.EncodeAll(raw, make([]byte, 0, len(raw)/2))
	return &EncodedHashes{
		Format:  FormatZstdJSON,
		Data:    data,
		RawSize: len(raw),
		Entries: len(hashes),
	}, nil
}

// DecodeHashes decompresses and unmarshals stored file hashes.
// A zero format with empty data represents the absence of hashes and returns (nil, nil).
func DecodeHashes(format uint8, data []byte) (map[string]string, error) {
	if format == 0 && len(data) == 0 {
		return nil, nil
	}
	if format != FormatZstdJSON {
		return nil, fmt.Errorf("unsupported file hashes format: %d", format)
	}
	raw, err := zstdDecoder.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("decompress file hashes: %w", err)
	}
	var m map[string]string
	if err := sonic.ConfigStd.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal file hashes: %w", err)
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}
