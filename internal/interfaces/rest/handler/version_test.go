package handler

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUTF8Runes(t *testing.T) {
	t.Run("keeps ascii rune limit", func(t *testing.T) {
		input := strings.Repeat("a", 20010)
		got := truncateUTF8Runes(input, 20000)
		if utf8.RuneCountInString(got) != 20000 {
			t.Fatalf("expected rune count 20000, got %d", utf8.RuneCountInString(got))
		}
		if !utf8.ValidString(got) {
			t.Fatal("expected valid utf8 string")
		}
	})

	t.Run("avoids splitting multibyte rune", func(t *testing.T) {
		input := strings.Repeat("你", 20001)
		got := truncateUTF8Runes(input, 20000)
		if len(got) >= len(input) {
			t.Fatalf("expected truncated string, got len %d", len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatal("expected valid utf8 string")
		}
		if got != strings.Repeat("你", 20000) {
			t.Fatalf("unexpected truncate result len=%d", len(got))
		}
	})
}
