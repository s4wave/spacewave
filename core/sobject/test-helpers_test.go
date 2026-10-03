package sobject

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const mockSharedObjectID = "test_object"

// mockConfigHash stands in for a config chain head in configs built by hand.
var mockConfigHash = bytes.Repeat([]byte{0xc0}, 32)

// mockPrevOpHash stands in for a previous operation the state does not hold.
var mockPrevOpHash = bytes.Repeat([]byte{0xa0}, 32)

// linkAt returns state's next link for the author of priv, moved to nonce.
// A nil state yields a link with no other heads.
func linkAt(state *SOState, priv crypto.PrivKey, nonce uint64) *SOOperationLink {
	// Start from the author's next link.
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		panic(err)
	}
	link := &SOOperationLink{}
	if state != nil {
		link, err = state.NextOperationLink(mockSharedObjectID, peerID.String())
		if err != nil {
			panic(err)
		}
	}
	if len(link.ConfigHash) == 0 {
		link.ConfigHash = mockConfigHash
	}

	// Move the link to nonce, naming a predecessor after the first.
	if nonce == link.Nonce {
		return link
	}
	link.Nonce = nonce
	switch {
	case nonce <= 1:
		link.PrevOpHash = nil
	case len(link.PrevOpHash) == 0:
		link.PrevOpHash = mockPrevOpHash
	}
	return link
}

func createMockPeers(t *testing.T, count uint64) []peer.Peer {
	t.Helper()

	peers := make([]peer.Peer, count)
	for i := range count {
		p, err := peer.NewPeer(nil)
		if err != nil {
			t.Fatalf("create peer %d: %v", i+1, err)
		}
		peers[i] = p
	}
	return peers
}

// mustPrivKeys returns the private key of each peer.
func mustPrivKeys(t *testing.T, peers []peer.Peer) []crypto.PrivKey {
	t.Helper()
	keys := make([]crypto.PrivKey, len(peers))
	for i, p := range peers {
		key, err := p.GetPrivKey(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = key
	}
	return keys
}

func mustMarshalVT[T interface{ MarshalVT() ([]byte, error) }](
	t *testing.T,
	msg T,
) []byte {
	t.Helper()
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}

// newTestSOState returns a valid state whose genesis config, signed by the
// first peer, gives each peer its role in roles: OWNER for the first peer and
// WRITER for the others when roles ends early. Key epoch 0 is granted to every
// reader. It also returns the genesis config change.
func newTestSOState(t *testing.T, peers []peer.Peer, roles ...SOParticipantRole) (*SOState, *SOConfigChange) {
	// Name each peer with its role.
	t.Helper()
	participants := make([]*SOParticipantConfig, len(peers))
	for i, p := range peers {
		role := SOParticipantRole_SOParticipantRole_WRITER
		if i == 0 {
			role = SOParticipantRole_SOParticipantRole_OWNER
		}
		if i < len(roles) {
			role = roles[i]
		}
		participants[i] = &SOParticipantConfig{PeerId: p.GetPeerID().String(), Role: role}
	}

	// Sign the genesis config as the first peer.
	owner := mustPrivKeys(t, peers[:1])[0]
	initial := &SharedObjectConfig{Participants: participants}
	genesis, err := BuildSOConfigChange(mockSharedObjectID, initial, initial, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS, owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := VerifyConfigChange(mockSharedObjectID, initial, genesis)
	if err != nil {
		t.Fatal(err)
	}

	// Grant key epoch 0 to every reader and sign the empty genesis checkpoint.
	_, epoch, err := RotateTransformKey(owner, mockSharedObjectID, participants, 0)
	if err != nil {
		t.Fatal(err)
	}
	epoch.Epoch = 0
	checkpoint, err := BuildGenesisSOCheckpoint(owner, mockSharedObjectID, cfg.GetConfigChainHash(), nil)
	if err != nil {
		t.Fatal(err)
	}
	state := &SOState{Config: cfg, Checkpoint: checkpoint, KeyEpochs: []*SOKeyEpoch{epoch}}
	if err := state.Validate(mockSharedObjectID); err != nil {
		t.Fatal(err)
	}
	return state, genesis
}

// testStepFactorySet returns the step factories of the default block transform.
func testStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	return sfs
}

// testHandle returns the snapshot of state as the participant priv, resolving
// earlier configs from history.
func testHandle(t *testing.T, state *SOState, priv crypto.PrivKey, history ...*SOConfigChange) *SOStateParticipantHandle {
	// Index the retained config changes by hash.
	t.Helper()
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	entries := make(map[string]*SOConfigChange, len(history))
	for _, entry := range history {
		h, err := HashSOConfigChange(entry)
		if err != nil {
			t.Fatal(err)
		}
		entries[hex.EncodeToString(h)] = entry
	}

	// Wrap the state in a participant handle.
	le := logrus.New().WithField("test", t.Name())
	return NewSOStateParticipantHandle(le, testStepFactorySet(), mockSharedObjectID, state, priv, peerID).
		WithConfigHistory(func(_ context.Context, h []byte) (*SOConfigChange, error) {
			return entries[hex.EncodeToString(h)], nil
		})
}

// writeTestOp encrypts data with the current key, signs it as priv at the
// author's next link and adds it to state.
func writeTestOp(t *testing.T, state *SOState, priv crypto.PrivKey, data string) *SOOperation {
	// Encrypt with the key of the current epoch. Empty data writes an
	// acknowledgment, which carries none.
	t.Helper()
	var enc []byte
	if data != "" {
		xfrm, err := testHandle(t, state, priv).GetTransformer(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		enc, err = xfrm.EncodeBlock([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
	}

	// Sign at the author's next link and add it.
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	link, err := state.NextOperationLink(mockSharedObjectID, peerID.String())
	if err != nil {
		t.Fatal(err)
	}
	op, err := BuildSOOperation(mockSharedObjectID, priv, enc, link, NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddOperation(mockSharedObjectID, op); err != nil {
		t.Fatal(err)
	}
	return op
}
