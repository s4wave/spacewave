package os_image

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestManifestSignature verifies that a manifest names its signer, and that
// rewriting the image or re-signing with another key changes what Verify
// reports, so the boot executable's pinned-signer check sees the rewrite.
func TestManifestSignature(t *testing.T) {
	// Sign a manifest for one image root with the owner's key.
	owner, ownerID := newKey(t)
	m := &OSRootManifest{Image: imageRef(t, "image one")}
	if err := m.Sign(owner); err != nil {
		t.Fatal(err)
	}
	signer, err := m.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if signer != ownerID {
		t.Fatalf("signer %s, want owner %s", signer, ownerID)
	}

	// A decoded copy verifies, as on the card.
	data, err := m.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	decoded := &OSRootManifest{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if signer, err := decoded.Verify(); err != nil || signer != ownerID {
		t.Fatalf("decoded manifest: signer %s, err %v", signer, err)
	}

	// Pointing the signed manifest at another image breaks the signature.
	rewritten := m.CloneVT()
	rewritten.Image = imageRef(t, "image two")
	if _, err := rewritten.Verify(); err == nil {
		t.Fatal("rewritten image verified under the owner's signature")
	}

	// Re-signing with another key verifies, but names that key.
	other, otherID := newKey(t)
	if err := rewritten.Sign(other); err != nil {
		t.Fatal(err)
	}
	if signer, err := rewritten.Verify(); err != nil || signer != otherID {
		t.Fatalf("re-signed manifest: signer %s, err %v", signer, err)
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

// imageRef returns a root reference for content.
func imageRef(t *testing.T, content string) *bucket.ObjectRef {
	h, err := hash.Sum(hash.RecommendedHashType, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return &bucket.ObjectRef{BucketId: "os-image", RootRef: block.NewBlockRef(h)}
}
