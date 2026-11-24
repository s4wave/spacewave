package os_image

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestImageAuthorization verifies that an authorization carries its image and
// signer through the card's packed text, that only a pinned signer's
// unaltered authorization verifies, so a rewritten image never boots, and that
// an image without a transform cannot be signed.
func TestImageAuthorization(t *testing.T) {
	// Authorize one image with the owner's key.
	owner, ownerID := newKey(t)
	image := newImage(t, "image one")
	signed, err := SignImage(owner, image)
	if err != nil {
		t.Fatal(err)
	}

	// The packed text on the card decodes to the same image and signer.
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
	got, signer, err := VerifyImage(decoded, []peer.ID{ownerID})
	if err != nil {
		t.Fatal(err)
	}
	if signer != ownerID || !got.EqualVT(image) {
		t.Fatalf("verified an image by %s, want the signed image by %s", signer, ownerID)
	}

	// Another peer's valid authorization is refused against the owner's pin.
	other, _ := newKey(t)
	foreign, err := SignImage(other, image)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyImage(foreign, []peer.ID{ownerID}); err == nil {
		t.Fatal("an unpinned signer's authorization verified")
	}

	// Pointing the owner's authorization at another root or another key
	// breaks it.
	rekeyed := image.CloneVT()
	rekeyed.TransformConf = newImage(t, "image one").GetTransformConf()
	for name, rewrite := range map[string]*bucket.ObjectRef{"root": newImage(t, "image two"), "transform": rekeyed} {
		rewritten := signed.CloneVT()
		rewritten.Data, err = rewrite.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := VerifyImage(rewritten, []peer.ID{ownerID}); err == nil {
			t.Fatalf("a rewritten %s verified under the owner's signature", name)
		}
	}

	// An image read with no transform is refused before signing.
	bare := &bucket.ObjectRef{RootRef: image.GetRootRef()}
	if _, err := SignImage(owner, bare); err == nil {
		t.Fatal("signed an image without a transform")
	}
}

// TestInstallCardRefuses verifies that an install writes a card only for the
// image's own root and transform and a grant any machine's new key can
// redeem, and that a refused install leaves no card file behind.
func TestInstallCardRefuses(t *testing.T) {
	// Authorize one image and build a grant the owner signs.
	owner, ownerID := newKey(t)
	image := newImage(t, "image one")
	signed, err := SignImage(owner, image)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	grant := func(terms *sobject.SOInvite) *sobject.SOInviteMessage {
		msg, _, err := sobject.BuildSOInviteMessage("space", owner, "local", terms)
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}
	open := grant(&sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER})
	unsigned := open.CloneVT()
	unsigned.Signature = nil

	// Each bad input is refused before any card file is written.
	rekeyed := image.CloneVT()
	rekeyed.TransformConf = newImage(t, "image one").GetTransformConf()
	cases := map[string]struct {
		image *bucket.ObjectRef
		grant *sobject.SOInviteMessage
	}{
		"another root":      {newImage(t, "image two"), open},
		"another transform": {rekeyed, open},
		"unsigned grant":    {image, unsigned},
		"targeted grant":    {image, grant(&sobject.SOInvite{TargetPeerId: ownerID.String()})},
		"expired grant":     {image, grant(&sobject.SOInvite{ExpiresAt: timestamp.New(now.Add(-time.Minute))})},
	}
	for name, c := range cases {
		dir := t.TempDir()
		if err := InstallCard(dir, []peer.ID{ownerID}, signed, c.grant, c.image, now); err == nil {
			t.Fatalf("%s: installed", name)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("%s: left %d card files", name, len(entries))
		}
	}

	// The good card round-trips through its files.
	dir := t.TempDir()
	if err := InstallCard(dir, []peer.ID{ownerID}, signed, open, image, now); err != nil {
		t.Fatal(err)
	}
	read, err := ReadGrant(filepath.Join(dir, CardGrant))
	if err != nil || read.GetInviteId() != open.GetInviteId() {
		t.Fatalf("grant %v err=%v", read, err)
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

// newImage returns an image whose root hashes content, under a fresh
// transform.
func newImage(t *testing.T, content string) *bucket.ObjectRef {
	h, err := hash.Sum(hash.RecommendedHashType, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	transform, err := NewTransform()
	if err != nil {
		t.Fatal(err)
	}
	return &bucket.ObjectRef{RootRef: block.NewBlockRef(h), TransformConf: transform}
}
