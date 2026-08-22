package store_kvtx_redis

import "testing"

// TestIsBasicRune tests that IsBasicRune accepts exactly 0-9, A-Z, and a-z.
func TestIsBasicRune(t *testing.T) {
	for c := range 256 {
		want := (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
		if got := IsBasicRune(byte(c)); got != want {
			t.Fatalf("IsBasicRune(%d %q) = %v, want %v", c, string(rune(c)), got, want)
		}
	}
}
