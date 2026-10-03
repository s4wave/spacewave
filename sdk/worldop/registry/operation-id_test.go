package s4wave_worldop_registry

import (
	"testing"

	"github.com/s4wave/spacewave/net/hash"
)

// TestPinnedOperationIdentity preserves executable and handler identity through history encoding.
func TestPinnedOperationIdentity(t *testing.T) {
	// Hash the artifact used by the pinned operation identity.
	root, err := hash.Sum(hash.RecommendedHashType, []byte("exact artifact"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the operation identity preserves the plugin, manifest, and handler.
	id := PinnedOperationID("colors", root.MarshalString(), "colors/like")
	plugin, manifest, handler, ok := ParsePinnedOperationID(id)
	if !ok || plugin != "colors" || manifest != root.MarshalString() || handler != "colors/like" {
		t.Fatalf("decoded identity = %q, %q, %q, %t", plugin, manifest, handler, ok)
	}

	// Verify an invalid manifest hash cannot identify an executable operation.
	if _, _, _, ok := ParsePinnedOperationID("plugin-op/colors/not-a-hash/colors%2Flike"); ok {
		t.Fatal("invalid executable identity was accepted")
	}
}
