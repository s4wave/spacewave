package boot

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	os_image "github.com/s4wave/spacewave/os/image"
)

// TestAuthorizedRootPin checks that first boot pins the card's signers and
// that a card rewritten afterwards, signers and authorization both, is refused.
func TestAuthorizedRootPin(t *testing.T) {
	// Write an owner's card for one root.
	card, data := t.TempDir(), t.TempDir()
	_, owner, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	h, err := hash.Sum(hash.RecommendedHashType, []byte("image root"))
	if err != nil {
		t.Fatal(err)
	}
	root := block.NewBlockRef(h)
	if err := os_image.WriteCard(card, owner, root); err != nil {
		t.Fatal(err)
	}

	// First boot pins the owner and serves the authorized root.
	got, err := AuthorizedRoot(card, data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.EqualsRef(root) {
		t.Fatalf("authorized root %s, want %s", got.MarshalString(), root.MarshalString())
	}

	// A card rewritten by another key after the pin authorizes nothing.
	_, other, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	if err := os_image.WriteCard(card, other, root); err != nil {
		t.Fatal(err)
	}
	if _, err := AuthorizedRoot(card, data); err == nil {
		t.Fatal("a card rewritten after the pin was accepted")
	}

	// The owner's authorization still verifies against the pin.
	if err := os_image.WriteCard(card, owner, root); err != nil {
		t.Fatal(err)
	}
	if _, err := AuthorizedRoot(card, data); err != nil {
		t.Fatal(err)
	}
}
