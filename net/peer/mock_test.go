package peer

import (
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// BuildMockKeys builds the set of mock keys that are expected to work.
func BuildMockKeys(t *testing.T) []crypto.PrivKey {
	// Generate the Ed25519 key used by the test cases.
	edPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Return the generated key as the supported test set.
	return []crypto.PrivKey{edPriv}
}
