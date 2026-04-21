package randstring

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/aperturerobotics/util/prng"
)

func TestRandString(t *testing.T) {
	rnd := rand.New(prng.BuildSeededRand([]byte("testing randstring"))) //nolint:gosec
	strs := make([]string, 10)
	for i := range strs {
		strs[i] = RandString(rnd, 8)
	}
	expected := []string{"EaTsEKPw", "nvvjMxQe", "JcnevYoi", "qhjIFzMl", "nIhGmfAT", "WCMJUZhe", "WAqmjYbL", "CgYZxfxR", "KphaTnzC", "iZDnLFYn"}
	if !slices.Equal(strs, expected) {
		t.Logf("expected: %#v", expected)
		t.Logf("actual: %#v", strs)
		t.FailNow()
	}
}
