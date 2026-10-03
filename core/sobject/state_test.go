package sobject

import (
	"bytes"
	"slices"
	"testing"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	blockenc_conf "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// TestBuildGenesisSOState checks that a new shared object's state is valid,
// grants its key to the owner and holds the initial state in its checkpoint.
func TestBuildGenesisSOState(t *testing.T) {
	// Build a genesis holding initial state data.
	peers := createMockPeers(t, 1)
	owner := mustPrivKeys(t, peers)[0]
	state, genesis, err := BuildGenesisSOState(nil, testStepFactorySet(), mockSharedObjectID, owner, []byte("initial"))
	if err != nil {
		t.Fatal(err)
	}

	// The owner is the sole participant of the genesis config.
	if genesis.GetChangeType() != SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS {
		t.Fatalf("change type %v; want GENESIS", genesis.GetChangeType())
	}
	participants := state.GetConfig().GetParticipants()
	if len(participants) != 1 || participants[0].GetPeerId() != peers[0].GetPeerID().String() || !IsOwner(participants[0].GetRole()) {
		t.Fatalf("genesis participants %v; want the owner alone", participants)
	}
	if err := state.ValidateAuthority(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}

	// The owner decodes the initial state from the height 0 checkpoint.
	checkpoint, err := testHandle(t, state, owner).GetCheckpoint(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.GetHeight() != 0 || string(checkpoint.GetStateData()) != "initial" {
		t.Fatalf("checkpoint at height %d holds %q", checkpoint.GetHeight(), checkpoint.GetStateData())
	}
}

// TestSOGrantValidateSignatureAllowsSelfSignedParticipantGrant checks that a
// participant may sign its own grant.
func TestSOGrantValidateSignatureAllowsSelfSignedParticipantGrant(t *testing.T) {
	// A participant encrypts a grant to itself, and the signature validates.
	p := createMockPeers(t, 1)[0]
	priv := mustPrivKeys(t, []peer.Peer{p})[0]
	pub, err := p.GetPeerID().ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract peer pubkey: %v", err)
	}
	grant, err := EncryptSOGrant(
		priv,
		pub,
		mockSharedObjectID,
		&SOGrantInner{
			TransformConf: &block_transform.Config{
				Steps: []*block_transform.StepConfig{{
					Id: blockenc_conf.ConfigID,
					Config: mustMarshalVT(t, &blockenc_conf.Config{
						BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
						Key:      []byte("0123456789abcdef0123456789abcdef"),
					}),
				}},
			},
		},
	)
	if err != nil {
		t.Fatalf("EncryptSOGrant: %v", err)
	}
	err = grant.ValidateSignature(mockSharedObjectID, []*SOParticipantConfig{{
		PeerId: grant.GetPeerId(),
		Role:   SOParticipantRole_SOParticipantRole_READER,
	}})
	if err != nil {
		t.Fatalf("ValidateSignature: %v", err)
	}
}

func TestBuildSOOperation(t *testing.T) {
	priv := mustPrivKeys(t, createMockPeers(t, 1))[0]
	for _, test := range []struct {
		name  string
		data  []byte
		nonce uint64
		ok    bool
	}{
		{"valid", []byte("test operation"), 1, true},
		{"acknowledgment", nil, 1, true},
		{"zero nonce", []byte("test operation"), 0, false},
		{"oversized data", make([]byte, MaxInnerDataSize+1), 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			op, err := BuildSOOperation(mockSharedObjectID, priv, test.data, linkAt(nil, priv, test.nonce), NewSOOperationLocalID())
			if (err == nil) != test.ok {
				t.Fatalf("BuildSOOperation error %v; want ok %v", err, test.ok)
			}
			if test.ok && (len(op.GetInner()) == 0 || op.GetSignature() == nil) {
				t.Fatal("built an operation without a body or signature")
			}
		})
	}
}

// TestNextOperationLink checks that each author extends its own chain and
// names every other head.
func TestNextOperationLink(t *testing.T) {
	// Start from a fresh state.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	ids := []string{peers[0].GetPeerID().String(), peers[1].GetPeerID().String()}
	link, err := state.NextOperationLink(mockSharedObjectID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if link.Nonce != 1 || len(link.PrevOpHash) != 0 || len(link.ParentHashes) != 0 {
		t.Fatalf("first link %v; want nonce 1 and no heads", link)
	}
	if !bytes.Equal(link.ConfigHash, state.GetConfig().GetConfigChainHash()) {
		t.Fatal("link does not name the current config")
	}

	// After one write each, the owner extends its head and names the writer's.
	first := writeTestOp(t, state, keys[0], "a")
	other := writeTestOp(t, state, keys[1], "b")
	link, err = state.NextOperationLink(mockSharedObjectID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if link.Nonce != 2 || !bytes.Equal(link.PrevOpHash, first.Hash()) {
		t.Fatalf("second link %v; want nonce 2 after the first", link)
	}
	if len(link.ParentHashes) != 1 || !bytes.Equal(link.ParentHashes[0], other.Hash()) {
		t.Fatal("second link does not name the other author's head")
	}
}

// TestAddOperation checks admission into the operation set: authentic
// operations enter once, in hash order, and covered ones stay out.
func TestAddOperation(t *testing.T) {
	// Start a state with two participants.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)

	// A held operation is not added twice.
	op := writeTestOp(t, state, keys[0], "a")
	if added, err := state.AddOperation(mockSharedObjectID, op); err != nil || added {
		t.Fatalf("re-adding a held operation: added %v, err %v", added, err)
	}

	// An operation bound to another object, or signed by someone other than
	// its named author, is rejected.
	other, err := BuildSOOperation("other_object", keys[1], []byte("x"), linkAt(nil, keys[1], 1), NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddOperation(mockSharedObjectID, other); err == nil {
		t.Fatal("added an operation bound to another object")
	}
	forged := mustMarshalVT(t, &SOOperationInner{
		PeerId:          peers[1].GetPeerID().String(),
		LocalId:         NewSOOperationLocalID(),
		Nonce:           1,
		OpData:          []byte("x"),
		SharedObjectId:  mockSharedObjectID,
		ProtocolVersion: SOOperationProtocolVersion,
		ConfigHash:      mockConfigHash,
	})
	sig, err := peer.NewSignature(SOOperationSignatureContext, keys[0], hash.RecommendedHashType, forged, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddOperation(mockSharedObjectID, &SOOperation{Inner: forged, Signature: sig}); err == nil {
		t.Fatal("added an operation signed under another author")
	}

	// Concurrent operations are held sorted by hash and validate.
	writeTestOp(t, state, keys[1], "b")
	writeTestOp(t, state, keys[1], "c")
	if !slices.IsSortedFunc(state.GetOps(), func(a, b *SOOperation) int { return bytes.Compare(a.Hash(), b.Hash()) }) {
		t.Fatal("operations are not sorted by hash")
	}
	if err := state.Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}

	// Once a checkpoint covers the set, its operations are not added again.
	held := slices.Clone(state.GetOps())
	advanceTestCheckpoint(t, state, keys[0])
	for _, op := range held {
		if added, err := state.AddOperation(mockSharedObjectID, op); err != nil || added {
			t.Fatalf("re-adding a covered operation: added %v, err %v", added, err)
		}
	}
}

// TestValidateRejectsMalformedState checks the structural rules Validate
// enforces on held operations and grants.
func TestValidateRejectsMalformedState(t *testing.T) {
	// Start a state holding one operation of each participant.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	base, _ := newTestSOState(t, peers, SOParticipantRole_SOParticipantRole_OWNER, SOParticipantRole_SOParticipantRole_WRITER)
	writeTestOp(t, base, keys[0], "a")
	writeTestOp(t, base, keys[1], "b")

	// Each malformed state fails validation.
	for _, test := range []struct {
		name   string
		mutate func(*SOState)
	}{
		{"unsorted operations", func(s *SOState) { slices.Reverse(s.Ops) }},
		{"duplicate operation", func(s *SOState) { s.Ops = append(s.Ops, s.Ops[0]) }},
		{"grant to a non-participant", func(s *SOState) {
			s.Config.Participants = s.Config.Participants[:1]
		}},
		{"unsorted key epochs", func(s *SOState) {
			s.KeyEpochs = append(s.KeyEpochs, s.KeyEpochs[0].CloneVT())
		}},
		{"checkpoint for another object", func(s *SOState) {
			checkpoint, err := BuildGenesisSOCheckpoint(keys[0], "other_object", s.GetConfig().GetConfigChainHash(), nil)
			if err != nil {
				t.Fatal(err)
			}
			s.Checkpoint = checkpoint
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := base.CloneVT()
			test.mutate(state)
			if err := state.Validate(mockSharedObjectID); err == nil {
				t.Fatal("validated a malformed state")
			}
		})
	}
}

// TestAdoptCheckpoint checks that a state adopts only an owner-signed
// checkpoint that descends from the held one, and drops what it covers.
func TestAdoptCheckpoint(t *testing.T) {
	// Hold one operation from each of an owner and a writer.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	base, _ := newTestSOState(t, peers)
	writeTestOp(t, base, keys[0], "a")
	writeTestOp(t, base, keys[1], "b")

	// A writer cannot sign a checkpoint.
	next, err := base.BuildNextCheckpoint(mockSharedObjectID, keys[1], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.CloneVT().AdoptCheckpoint(mockSharedObjectID, next); err == nil {
		t.Fatal("adopted a checkpoint a writer signed")
	}

	// The owner's checkpoint covers both operations; a later one stays.
	state := base.CloneVT()
	next, err = state.BuildNextCheckpoint(mockSharedObjectID, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	later := writeTestOp(t, state, keys[1], "c")
	if err := state.AdoptCheckpoint(mockSharedObjectID, next); err != nil {
		t.Fatal(err)
	}
	if len(state.GetOps()) != 1 || !bytes.Equal(state.GetOps()[0].Hash(), later.Hash()) {
		t.Fatalf("adoption left %d operations; want only the later one", len(state.GetOps()))
	}
	if err := state.Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}

	// The held checkpoint and the genesis below it are ignored.
	if err := state.AdoptCheckpoint(mockSharedObjectID, next); err != nil {
		t.Fatalf("re-adopting the held checkpoint: %v", err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, base.GetCheckpoint()); err != nil || !bytes.Equal(state.GetCheckpoint().Hash(), next.Hash()) {
		t.Fatalf("adopting a lower checkpoint replaced the held one: %v", err)
	}

	// A different checkpoint at the held height conflicts.
	fork := base.CloneVT()
	writeTestOp(t, fork, keys[0], "d")
	conflict, err := fork.BuildNextCheckpoint(mockSharedObjectID, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, conflict); err == nil {
		t.Fatal("adopted a conflicting checkpoint at the held height")
	}

	// A checkpoint one height up must follow the held one.
	stray, err := fork.CloneVT().BuildNextCheckpoint(mockSharedObjectID, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fork.AdoptCheckpoint(mockSharedObjectID, stray); err != nil {
		t.Fatal(err)
	}
	stray, err = fork.BuildNextCheckpoint(mockSharedObjectID, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, stray); err == nil {
		t.Fatal("adopted a checkpoint that does not follow the held one")
	}
}

// advanceTestCheckpoint signs and adopts, as the owner priv, a checkpoint
// covering every operation state holds.
func advanceTestCheckpoint(t *testing.T, state *SOState, priv crypto.PrivKey) {
	t.Helper()
	next, err := state.BuildNextCheckpoint(mockSharedObjectID, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, next); err != nil {
		t.Fatal(err)
	}
}
