package os_image

import (
	"testing"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestImageRootAuthorization verifies that an authorization carries its root
// and signer through the card's packed text, and that only a pinned signer's
// unaltered authorization verifies, so a rewritten image never boots.
func TestImageRootAuthorization(t *testing.T) {
	// Authorize one image root with the owner's key.
	owner, ownerID := newKey(t)
	root := imageRoot(t, "image one")
	signed, err := SignImageRoot(owner, root)
	if err != nil {
		t.Fatal(err)
	}

	// The packed text on the card decodes to the same root and signer.
	data, err := signed.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	unpacked, ok := packedmsg.DecodePackedMessage(packedmsg.EncodePackedMessage(data))
	if !ok {
		t.Fatal("packed authorization did not decode")
	}
	decoded, err := peer.UnmarshalSignedMsg(unpacked)
	if err != nil {
		t.Fatal(err)
	}
	got, signer, err := VerifyImageRoot(decoded, []peer.ID{ownerID})
	if err != nil {
		t.Fatal(err)
	}
	if signer != ownerID || !got.EqualsRef(root) {
		t.Fatalf("verified a root by %s, want the signed root by %s", signer, ownerID)
	}

	// Another peer's valid authorization is refused against the owner's pin.
	other, _ := newKey(t)
	foreign, err := SignImageRoot(other, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyImageRoot(foreign, []peer.ID{ownerID}); err == nil {
		t.Fatal("an unpinned signer's authorization verified")
	}

	// Pointing the owner's authorization at another root breaks it.
	rewritten := signed.CloneVT()
	rewritten.Data, err = imageRoot(t, "image two").MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyImageRoot(rewritten, []peer.ID{ownerID}); err == nil {
		t.Fatal("a rewritten root verified under the owner's signature")
	}
}

// newKey returns a fresh signing key and its peer ID.
func newKey(t *testing.T) (crypto.PrivKey, peer.ID) {
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := p.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return key, p.GetPeerID()
}

// imageRoot returns a root block reference for content.
func imageRoot(t *testing.T, content string) *block.BlockRef {
	h, err := hash.Sum(hash.RecommendedHashType, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return block.NewBlockRef(h)
}
