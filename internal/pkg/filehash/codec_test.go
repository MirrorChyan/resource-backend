package filehash

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	hashes := make(map[string]string, 200)
	for i := range 200 {
		hashes[fmt.Sprintf("assets/resource/textures/item_%04d.png", i)] = strings.Repeat(fmt.Sprintf("%x", i%16), 64)
	}

	enc, err := EncodeHashes(hashes)
	require.NoError(t, err)
	require.NotNil(t, enc)
	require.Equal(t, FormatZstdJSON, enc.Format)
	require.Equal(t, len(hashes), enc.Entries)
	require.Greater(t, enc.RawSize, len(enc.Data))

	// Deterministic output: encoding twice yields identical bytes (sorted JSON keys).
	enc2, err := EncodeHashes(hashes)
	require.NoError(t, err)
	require.True(t, bytes.Equal(enc.Data, enc2.Data))

	decoded, err := DecodeHashes(enc.Format, enc.Data)
	require.NoError(t, err)
	require.Equal(t, hashes, decoded)
}

func TestEncodeEmptyReturnsNil(t *testing.T) {
	enc, err := EncodeHashes(nil)
	require.NoError(t, err)
	require.Nil(t, enc)

	enc, err = EncodeHashes(map[string]string{})
	require.NoError(t, err)
	require.Nil(t, enc)

	dec, err := DecodeHashes(0, nil)
	require.NoError(t, err)
	require.Nil(t, dec)
}

func TestDecodeRejectsCorruptedData(t *testing.T) {
	_, err := DecodeHashes(99, []byte("anything"))
	require.ErrorContains(t, err, "unsupported file hashes format")

	_, err = DecodeHashes(FormatZstdJSON, []byte("not-valid-zstd"))
	require.ErrorContains(t, err, "decompress")

	badJSON := zstdDefaultEncoder.EncodeAll([]byte("{not-json"), nil)
	_, err = DecodeHashes(FormatZstdJSON, badJSON)
	require.ErrorContains(t, err, "unmarshal")
}

func TestCodecConcurrentSafety(t *testing.T) {
	hashes := map[string]string{
		"a/b/c.bin": strings.Repeat("a", 64),
		"a/b/d.bin": strings.Repeat("b", 64),
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				enc, err := EncodeHashes(hashes)
				require.NoError(t, err)
				dec, err := DecodeHashes(enc.Format, enc.Data)
				require.NoError(t, err)
				require.Equal(t, hashes, dec)
			}
		}()
	}
	wg.Wait()
}
