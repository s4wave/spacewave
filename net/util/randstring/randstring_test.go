package randstring

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/prng"
)

// TestRandString preserves the seeded caller's exact output sequence.
func TestRandString(t *testing.T) {
	// Generate consecutive strings from the same seeded source.
	rnd := rand.New(prng.BuildSeededRand([]byte("testing randstring"))) //nolint:gosec
	strs := make([]string, 10)
	for i := range strs {
		strs[i] = RandString(rnd, 8)
	}

	// Compare the complete sequence, including generator advancement.
	expected := []string{"EaTsEKPw", "nvvjMxQe", "JcnevYoi", "qhjIFzMl", "nIhGmfAT", "WCMJUZhe", "WAqmjYbL", "CgYZxfxR", "KphaTnzC", "iZDnLFYn"}
	if !slices.Equal(strs, expected) {
		t.Logf("expected: %#v", expected)
		t.Logf("actual: %#v", strs)
		t.FailNow()
	}
}

// TestRandomIdentifier checks the fresh-entropy path and identifier contract.
func TestRandomIdentifier(t *testing.T) {
	// Exercise empty strings and identifiers spanning multiple entropy batches.
	for _, length := range []int{0, 4, 8, 16, 100} {
		value := RandString(nil, length)
		if len(value) != length {
			t.Fatalf("length %d: got %d", length, len(value))
		}
		if strings.Trim(value, letterBytes) != "" {
			t.Fatalf("invalid alphabet: %q", value)
		}
	}

	// Catch a constant-output runtime regression without testing random collisions.
	first := RandomIdentifier(0)
	if len(first) != 8 || strings.Trim(first, "abcdefghijklmnopqrstuvwxyz") != "" {
		t.Fatalf("invalid default identifier: %q", first)
	}
	for range 10 {
		if RandomIdentifier(0) != first {
			return
		}
	}
	t.Fatal("all fresh identifiers repeated")
}
