package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// testObjectID is the isolated SharedObject identity used by migration fixtures.
const testObjectID = "01m343naxwrsfbj4c9bagcj82x"

// legacyState supplies old-format nonce heads, queues, and a real signed key grant.
func legacyState(t *testing.T) (*sobject.SOState, crypto.PrivKey) {
	// Generate an owner identity and an independently authenticated root grant.
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := sobject.EncryptSOGrant(key, key.GetPublic(), testObjectID, &sobject.SOGrantInner{TransformConf: &block_transform.Config{Steps: []*block_transform.StepConfig{{Id: "fixture", Config: []byte{1}}}}})
	if err != nil {
		t.Fatal(err)
	}

	// Sign a retained opaque root whose account head predates operation hashes.
	state := &sobject.SOState{
		Config:              &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{{PeerId: id.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER}}, ConfigChainHash: bytes.Repeat([]byte{7}, 32), ConfigChainSeqno: 14},
		Root:                &sobject.SORoot{Inner: []byte("opaque encrypted retained World root"), InnerSeqno: 19, AccountNonces: []*sobject.SOAccountNonce{{PeerId: id.String(), Nonce: 8}}},
		RootGrants:          []*sobject.SOGrant{grant},
		Ops:                 []*sobject.SOOperation{{Inner: []byte("obsolete operation")}},
		OpRejections:        []*sobject.SOPeerOpRejections{{PeerId: id.String()}},
		QueuedAccountNonces: []*sobject.SOAccountNonce{{PeerId: id.String(), Nonce: 9}},
	}
	if err := state.Root.SignInnerData(key, testObjectID, state.Root.InnerSeqno, hash.RecommendedHashType); err != nil {
		t.Fatal(err)
	}
	return state, key
}

// TestMigrateState verifies repair authority and preservation of retained state.
func TestMigrateState(t *testing.T) {
	// Confirm the fixture reproduces the old nonce-head validation failure.
	state, key := legacyState(t)
	before := state.CloneVT()
	if err := state.Validate(testObjectID); err == nil {
		t.Fatal("legacy state unexpectedly validates")
	}

	// Repair the clone using the locally authorized owner and validator.
	next, genesis, err := MigrateState(testObjectID, state, key, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Validate(testObjectID); err != nil {
		t.Fatal(err)
	}
	if err := validateRoot(testObjectID, next); err != nil {
		t.Fatal(err)
	}
	if err := sobject.VerifyConfigChain(testObjectID, []*sobject.SOConfigChange{genesis}); err != nil {
		t.Fatal(err)
	}

	// Preserve the source, opaque World bytes, nonce sequence, and key grant.
	if !state.EqualVT(before) {
		t.Fatal("migration mutated its input")
	}
	if !bytes.Equal(next.Root.Inner, before.Root.Inner) || next.Root.InnerSeqno != before.Root.InnerSeqno {
		t.Fatal("retained World root changed")
	}
	if len(next.Root.AccountNonces) != 1 || next.Root.AccountNonces[0].Nonce != 8 || next.Root.AccountNonces[0].PeerId != before.Root.AccountNonces[0].PeerId {
		t.Fatal("retained account nonce changed")
	}
	if len(next.RootGrants) != 1 || !next.RootGrants[0].EqualVT(before.RootGrants[0]) {
		t.Fatal("key grant changed")
	}
	if len(next.Ops)+len(next.OpRejections)+len(next.QueuedAccountNonces) != 0 {
		t.Fatal("obsolete pending records survived")
	}
	if next.Config.ConfigChainSeqno != 0 || genesis.SharedObjectId != testObjectID {
		t.Fatal("configuration was not reset to an object-bound genesis")
	}

	// Reject a private key that has no owner authority in the retained config.
	other, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := MigrateState(testObjectID, state, other, key); err == nil {
		t.Fatal("unauthorized owner was accepted")
	}
	if _, _, err := MigrateState(testObjectID, state, key, other); err == nil {
		t.Fatal("unauthorized validator was accepted")
	}
}

// TestMigratedHead fixes the sentinel encoding independently of its framing code.
func TestMigratedHead(t *testing.T) {
	// Compare a complete literal frame with the domain-separated sentinel.
	frame := []byte("spacewave/sobject/migrated-head\x00\x00\x00\x00\x02so\x00\x00\x00\x04peer\x00\x00\x00\x00\x00\x00\x00\x09")
	expected := sha256.Sum256(frame)
	if !bytes.Equal(MigratedHead("so", "peer", 9), expected[:]) {
		t.Fatal("sentinel byte encoding changed")
	}

	// Distinguish identities whose unframed concatenations would collide.
	if bytes.Equal(MigratedHead("ab", "c", 9), MigratedHead("a", "bc", 9)) {
		t.Fatal("identity framing is ambiguous")
	}
}
