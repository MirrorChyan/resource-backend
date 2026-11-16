package dispense

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignURL(t *testing.T) {
	const (
		prefix = "https://cdn.example.com"
		pk     = "test-private-key"
		ts     = int64(1794804150)
		nonce  = "3MArKi6KOGpKEreJ7tM10gdOB09"
	)

	tests := []struct {
		name     string
		relPath  string
		wirePath string
	}{
		{
			name:     "ascii path is unchanged",
			relPath:  "maa/12/windows-amd64/MAA-v5.0.0-win-x64.zip",
			wirePath: "/maa/12/windows-amd64/MAA-v5.0.0-win-x64.zip",
		},
		{
			name:     "non-ascii path is percent-encoded",
			relPath:  "foundation-sunshine/11761/windows-amd64/foundation-sunshine-v2026.1031.121458.杂鱼-win-x64.exe",
			wirePath: "/foundation-sunshine/11761/windows-amd64/foundation-sunshine-v2026.1031.121458.%E6%9D%82%E9%B1%BC-win-x64.exe",
		},
		{
			name:     "space is percent-encoded",
			relPath:  "res/1/any-any/my package.zip",
			wirePath: "/res/1/any-any/my%20package.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signURL(prefix, pk, tt.relPath, 42, ts, nonce)

			rest, ok := strings.CutPrefix(got, prefix)
			require.True(t, ok, got)
			path, rawQuery, ok := strings.Cut(rest, "?")
			require.True(t, ok, got)

			// clients send an ASCII-only URL as is, so this is the path the CDN hashes
			require.Equal(t, tt.wirePath, path)

			query, err := url.ParseQuery(rawQuery)
			require.NoError(t, err)
			require.Equal(t, "42", query.Get("r"))

			parts := strings.Split(query.Get("auth_key"), "-")
			require.Len(t, parts, 4)
			require.Equal(t, []string{"1794804150", nonce, "0"}, parts[:3])

			sum := md5.Sum([]byte(strings.Join([]string{path, parts[0], parts[1], parts[2], pk}, "-")))
			require.Equal(t, hex.EncodeToString(sum[:]), parts[3])
		})
	}
}
