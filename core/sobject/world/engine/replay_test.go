package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// replayTestObjectID is the SharedObject the replay tests' operations name.
const replayTestObjectID = "test-replay"

// replayTestSpace is a World and the signed operations written to it.
type replayTestSpace struct {
	t       *testing.T
	c       *Controller
	so      *testSharedObject
	genesis *InnerState
	// config is the only config the operations name.
	config *sobject.SharedObjectConfig
	// names labels operations by hash in failures.
	names map[string]string
}

// newReplayTestSpace builds a genesis World whose config lets writers write.
func newReplayTestSpace(t *testing.T, writers ...peer.ID) *replayTestSpace {
	// Build the genesis World.
	t.Helper()
	c, so, genesis := newProcessTestWorld(t, context.Background())

	// Name the writers in the config every operation names.
	config := &sobject.SharedObjectConfig{ConfigChainHash: bytes.Repeat([]byte{7}, 32)}
	for _, pid := range writers {
		config.Participants = append(config.Participants, &sobject.SOParticipantConfig{
			PeerId: pid.String(),
			Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
		})
	}
	return &replayTestSpace{t: t, c: c, so: so, genesis: genesis, config: config, names: map[string]string{}}
}

// sign signs a World operation creating key as the next operation of an author.
func (s *replayTestSpace) sign(name string, priv crypto.PrivKey, key string, link *sobject.SOOperationLink) *sobject.SOOperation {
	// Encode the object creation.
	s.t.Helper()
	tx, err := world_block_tx.NewTxCreateObject(key, s.genesis.GetHeadRef().CloneVT())
	if err != nil {
		s.t.Fatal(err.Error())
	}
	data := marshalApplyTxOpForProcessTest(s.t, tx)

	// Sign it under the Space config.
	link.ConfigHash = s.config.GetConfigChainHash()
	op, err := sobject.BuildSOOperation(replayTestObjectID, priv, data, link, sobject.NewSOOperationLocalID())
	if err != nil {
		s.t.Fatal(err.Error())
	}
	s.names[string(op.Hash())] = name
	return op
}

// member returns a member that replays on the given device. A cold member
// replays every delivery from genesis.
func (s *replayTestSpace) member(device peer.ID, cold bool) *replayTestMember {
	return &replayTestMember{
		space: s,
		set:   sobject.NewSOOperationSet(replayTestObjectID, &sobject.SOCheckpointInner{}),
		cold:  cold,
		snap:  &replayTestSnapshot{config: s.config},
		replayer: &replayer{
			c:    s.c,
			so:   &testSharedObject{peerID: device, blockStore: s.so.blockStore},
			base: s.genesis,
		},
	}
}

// replayTestSnapshot resolves the one config every operation names, whose
// participants are the current members.
type replayTestSnapshot struct {
	testSharedObjectSnapshot
	config *sobject.SharedObjectConfig
}

// GetParticipantConfigForPeer returns the member peerID of the config.
func (s *replayTestSnapshot) GetParticipantConfigForPeer(_ context.Context, peerID string) (*sobject.SOParticipantConfig, error) {
	for _, p := range s.config.GetParticipants() {
		if p.GetPeerId() == peerID {
			return p, nil
		}
	}
	return nil, sobject.ErrNotParticipant
}

// GetConfig returns the config.
func (s *replayTestSnapshot) GetConfig(context.Context) (*sobject.SharedObjectConfig, error) {
	return s.config, nil
}

// GetConfigByHash returns the config when hash names it.
func (s *replayTestSnapshot) GetConfigByHash(_ context.Context, hash []byte) (*sobject.SharedObjectConfig, error) {
	if !bytes.Equal(hash, s.config.GetConfigChainHash()) {
		return nil, errors.New("unknown config")
	}
	return s.config, nil
}

// replayTestMember is one device holding operations and replaying them.
type replayTestMember struct {
	space    *replayTestSpace
	set      *sobject.SOOperationSet
	snap     *replayTestSnapshot
	cold     bool
	replayer *replayer
}

// replayTestResult is the World and named outcomes after one replay.
type replayTestResult struct {
	state    *InnerState
	outcomes []string
}

// deliver adds a batch of operations and replays the set once.
func (m *replayTestMember) deliver(ops ...*sobject.SOOperation) replayTestResult {
	// Add the batch to the set.
	t := m.space.t
	t.Helper()
	for _, op := range ops {
		if _, err := m.set.Add(op); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Replay it, from genesis when cold.
	if m.cold {
		m.replayer.positions = nil
	}
	state, outcomes, err := m.replayer.replay(context.Background(), m.snap, m.set, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Name the outcomes.
	res := replayTestResult{state: state}
	for _, o := range outcomes {
		outcome := "applied"
		if o.reason != "" {
			outcome = o.reason
		}
		res.outcomes = append(res.outcomes, m.space.names[string(o.hash)]+": "+outcome)
	}
	return res
}

// TestReplayConvergesAcrossDeliveryOrders replays concurrent transactions from
// two members that receive them in opposite orders, cold and with a warm cache.
// Both members create the same object concurrently. Every member must reach
// the same World with the same outcomes: the creation that sorts first applies
// and the other is rejected.
func TestReplayConvergesAcrossDeliveryOrders(t *testing.T) {
	// Build the Space and its two member devices.
	privA, pidA := newReplayTestKey(t)
	privB, pidB := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA, pidB)

	// Member A creates its own object and then the shared one. Member B
	// creates the shared object concurrently.
	opA := space.sign("A own", privA, "object-a", &sobject.SOOperationLink{Nonce: 1})
	opAShared := space.sign("A shared", privA, "object-shared", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: opA.Hash()})
	opBShared := space.sign("B shared", privB, "object-shared", &sobject.SOOperationLink{Nonce: 1})

	// Each member delivers its own operations first, one at a time, then the
	// other member's operations as one batch.
	type run struct {
		name string
		res  replayTestResult
	}
	var runs []run
	for _, cold := range []bool{true, false} {
		mode := "warm"
		if cold {
			mode = "cold"
		}
		memberA := space.member(pidA, cold)
		memberA.deliver(opA)
		memberA.deliver(opAShared)
		runs = append(runs, run{"member A " + mode, memberA.deliver(opBShared)})

		memberB := space.member(pidB, cold)
		memberB.deliver(opBShared)
		runs = append(runs, run{"member B " + mode, memberB.deliver(opA, opAShared)})
	}

	// Every run reaches the same World with the same outcomes.
	want := runs[0]
	for _, got := range runs[1:] {
		if !got.res.state.EqualVT(want.res.state) || !slices.Equal(got.res.outcomes, want.res.outcomes) {
			t.Errorf("%s diverged from %s:\n  %q\n  %q", got.name, want.name, want.res.outcomes, got.res.outcomes)
		}
	}

	// Only the later of the two shared creations is rejected.
	set := sobject.NewSOOperationSet(replayTestObjectID, &sobject.SOCheckpointInner{})
	for _, op := range []*sobject.SOOperation{opA, opAShared, opBShared} {
		if _, err := set.Add(op); err != nil {
			t.Fatal(err.Error())
		}
	}
	order := set.Order()
	if len(want.res.outcomes) != len(order) {
		t.Fatalf("outcomes %q; want %d", want.res.outcomes, len(order))
	}
	for i, h := range order {
		applied := want.res.outcomes[i] == space.names[string(h)]+": applied"
		if wantApplied := i != 2; applied != wantApplied {
			t.Errorf("outcome %q; want applied=%v", want.res.outcomes[i], wantApplied)
		}
	}
}

// TestReplayRejectsAuthorOutsideConfig rejects an operation whose author
// cannot write under the config it names, on every replaying device.
func TestReplayRejectsAuthorOutsideConfig(t *testing.T) {
	// Build a Space whose only writer is A, and an operation by outsider C.
	_, pidA := newReplayTestKey(t)
	privC, pidC := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	opC := space.sign("C own", privC, "object-c", &sobject.SOOperationLink{Nonce: 1})

	// Both the writer and the outsider reject it.
	for _, device := range []peer.ID{pidA, pidC} {
		res := space.member(device, true).deliver(opC)
		want := []string{"C own: its author could not write to the shared object"}
		if !slices.Equal(res.outcomes, want) || !res.state.EqualVT(space.genesis) {
			t.Errorf("replay on %s: outcomes %q; want %q", device, res.outcomes, want)
		}
	}
}

// newReplayTestKey generates a device key and its peer ID.
func newReplayTestKey(t *testing.T) (crypto.PrivKey, peer.ID) {
	// Generate the device key.
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Derive the peer ID from the key.
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("derive peer id: %v", err)
	}
	return priv, pid
}
