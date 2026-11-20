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

// TestInstallCardRefuses verifies that an install writes a card only for the
// image's own root and a grant any machine's new key can redeem, and that a
// refused install leaves no card file behind.
func TestInstallCardRefuses(t *testing.T) {
	// Authorize one root and build a grant the owner signs.
	owner, ownerID := newKey(t)
	root := imageRoot(t, "image one")
	signed, err := SignImageRoot(owner, root)
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
	cases := map[string]struct {
		root  *block.BlockRef
		grant *sobject.SOInviteMessage
	}{
		"another root":   {imageRoot(t, "image two"), open},
		"unsigned grant": {root, unsigned},
		"targeted grant": {root, grant(&sobject.SOInvite{TargetPeerId: ownerID.String()})},
		"expired grant":  {root, grant(&sobject.SOInvite{ExpiresAt: timestamp.New(now.Add(-time.Minute))})},
	}
	for name, c := range cases {
		dir := t.TempDir()
		if err := InstallCard(dir, []peer.ID{ownerID}, signed, c.grant, c.root, now); err == nil {
			t.Fatalf("%s: installed", name)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("%s: left %d card files", name, len(entries))
		}
	}

	// The good card round-trips through its files.
	dir := t.TempDir()
	if err := InstallCard(dir, []peer.ID{ownerID}, signed, open, root, now); err != nil {
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

// imageRoot returns a root block reference for content.
func imageRoot(t *testing.T, content string) *block.BlockRef {
	h, err := hash.Sum(hash.RecommendedHashType, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	return block.NewBlockRef(h)
}
