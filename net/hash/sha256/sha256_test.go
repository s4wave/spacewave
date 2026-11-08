package sha256

import (
	"crypto/sha256"
	"testing"
)

// TestMatchesStdlib checks New and Sum256 against crypto/sha256.
func TestMatchesStdlib(t *testing.T) {
	data := make([]byte, 1000)
	for i := range data {
		data[i] = byte(i * 7)
	}
	for _, n := range []int{0, 1, 55, 56, 63, 64, 65, 119, 120, 128, 1000} {
		want := sha256.Sum256(data[:n])
		if got := Sum256(data[:n]); got != want {
			t.Fatalf("Sum256 of %d bytes: got %x, want %x", n, got, want)
		}
		h := New()
		_, _ = h.Write(data[:n])
		if got := h.Sum(nil); string(got) != string(want[:]) {
			t.Fatalf("New of %d bytes: got %x, want %x", n, got, want)
		}
	}
}
