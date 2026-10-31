package sobject_world_engine

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/world"
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
	return s.signTx(name, priv, tx, link)
}

// signTx signs a World transaction as the next operation of an author.
func (s *replayTestSpace) signTx(name string, priv crypto.PrivKey, tx *world_block_tx.Tx, link *sobject.SOOperationLink) *sobject.SOOperation {
	s.t.Helper()
	return s.signOp(name, priv, marshalApplyTxOpForProcessTest(s.t, tx), link)
}

// signRetainedRoot signs, as the next operation of an author, an operation
// retaining the genesis World under name.
func (s *replayTestSpace) signRetainedRoot(name string, priv crypto.PrivKey, root string, link *sobject.SOOperationLink) *sobject.SOOperation {
	s.t.Helper()
	data, err := (&SOWorldOp{Body: &SOWorldOp_SetRetainedRoot{
		SetRetainedRoot: &SetRetainedRootOp{Name: root, RootRef: s.genesis.GetHeadRef().GetRootRef()},
	}}).MarshalVT()
	if err != nil {
		s.t.Fatal(err.Error())
	}
	return s.signOp(name, priv, data, link)
}

// signOp signs World operation data as the next operation of an author.
func (s *replayTestSpace) signOp(name string, priv crypto.PrivKey, data []byte, link *sobject.SOOperationLink) *sobject.SOOperation {
	// Sign it under the Space config.
	s.t.Helper()
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
	// checkpoint and set, when set, are the checkpoint and operation set the
	// snapshot holds.
	checkpoint *sobject.SOCheckpointInner
	set        *sobject.SOOperationSet
}

// GetCheckpoint returns the held checkpoint.
func (s *replayTestSnapshot) GetCheckpoint(ctx context.Context) (*sobject.SOCheckpointInner, error) {
	if s.checkpoint != nil {
		return s.checkpoint, nil
	}
	return s.testSharedObjectSnapshot.GetCheckpoint(ctx)
}

// GetOperationSet returns the held operation set.
func (s *replayTestSnapshot) GetOperationSet(ctx context.Context) (*sobject.SOOperationSet, error) {
	if s.set != nil {
		return s.set, nil
	}
	return s.testSharedObjectSnapshot.GetOperationSet(ctx)
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
	// ops are the delivered operations.
	ops []*sobject.SOOperation
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
	m.ops = append(m.ops, ops...)

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

// adopt replaces the member's checkpoint with one at height whose author heads
// are heads and whose World is world, and syncs the replay to it.
func (m *replayTestMember) adopt(height uint64, world *InnerState, heads ...*sobject.SOOperation) {
	// Build the checkpoint.
	t := m.space.t
	t.Helper()
	data, err := world.MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	checkpoint := &sobject.SOCheckpointInner{Height: height, StateData: data}
	for _, op := range heads {
		inner, err := op.UnmarshalInner()
		if err != nil {
			t.Fatal(err.Error())
		}
		checkpoint.Authors = append(checkpoint.Authors, &sobject.SOOperationPosition{PeerId: inner.GetPeerId(), Nonce: inner.GetNonce(), OpHash: op.Hash()})
	}

	// Hold the operations above it, and sync.
	set := sobject.NewSOOperationSet(replayTestObjectID, checkpoint)
	for _, op := range m.ops {
		if _, err := set.Add(op); err != nil {
			t.Fatal(err.Error())
		}
	}
	m.set = set
	m.snap.checkpoint, m.snap.set = checkpoint, set
	if _, _, err := m.replayer.sync(context.Background(), m.snap, nil); err != nil {
		t.Fatal(err.Error())
	}
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

// TestReplayResumesSavedReplay saves a replay and resumes it in a new
// replayer, as a remounted World engine does. The resumed replayer must not
// replay the saved operations, must restore the World after each of them, and
// must reach the World a cold replay reaches when another operation arrives.
func TestReplayResumesSavedReplay(t *testing.T) {
	// Build the Space and its operations.
	privA, pidA := newReplayTestKey(t)
	privB, pidB := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA, pidB)
	opA := space.sign("A own", privA, "object-a", &sobject.SOOperationLink{Nonce: 1})
	opARoot := space.signRetainedRoot("A root", privA, "genesis", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: opA.Hash()})
	opAShared := space.sign("A shared", privA, "object-shared", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: opARoot.Hash()})
	opBShared := space.sign("B shared", privB, "object-shared", &sobject.SOOperationLink{Nonce: 1})

	// Member A replays its own operations and saves the replay.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	memberA := space.member(pidA, false)
	memberA.replayer.so.(*testSharedObject).localStore = store
	saved := memberA.deliver(opA, opARoot, opAShared)
	if err := memberA.replayer.save(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// A new replayer resumes the saved replay. Its snapshot resolves no
	// config, so replaying any saved operation would change its outcome.
	restored := &replayTestMember{
		space: space,
		set:   memberA.set,
		snap:  &replayTestSnapshot{config: &sobject.SharedObjectConfig{}},
		replayer: &replayer{
			c:  space.c,
			so: &testSharedObject{peerID: pidA, blockStore: space.so.blockStore, localStore: store},
		},
	}
	if err := restored.replayer.load(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if !slices.EqualFunc(restored.replayer.positions, memberA.replayer.positions, func(a, b replayPosition) bool {
		return a.state.EqualVT(b.state)
	}) {
		t.Fatal("restored positions hold Worlds other than the saved replay's")
	}
	resumed := restored.deliver()
	if !resumed.state.EqualVT(saved.state) || !slices.Equal(resumed.outcomes, saved.outcomes) {
		t.Fatalf("resumed %q; want %q", resumed.outcomes, saved.outcomes)
	}

	// After B's operation arrives, it matches a cold replay of all three.
	restored.snap = memberA.snap
	got := restored.deliver(opBShared)
	want := space.member(pidB, true).deliver(opA, opARoot, opAShared, opBShared)
	if !got.state.EqualVT(want.state) || !slices.Equal(got.outcomes, want.outcomes) {
		t.Errorf("resumed replay %q; want %q", got.outcomes, want.outcomes)
	}
}

// TestReplayResumesSavedInit resumes a saved replay of a World that its first
// operation initialized. The restored replay must hold the initialized World
// after every position, so that an operation placed after the first position
// applies instead of finding the World uninitialized.
func TestReplayResumesSavedInit(t *testing.T) {
	// Build the operations: A initializes the World and then writes to it.
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	initOp, err := NewInitWorldOp(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	initData, err := (&SOWorldOp{Body: &SOWorldOp_InitWorld{InitWorld: initOp}}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Sign them in a chain.
	opInit := space.signOp("A init", privA, initData, &sobject.SOOperationLink{Nonce: 1})
	opA := space.sign("A own", privA, "object-a", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: opInit.Hash()})
	opB := space.sign("A next", privA, "object-b", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: opA.Hash()})

	// Member A replays the initialization from an uninitialized checkpoint and
	// saves the replay.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	memberA := space.member(pidA, false)
	memberA.replayer.base = &InnerState{}
	memberA.replayer.so.(*testSharedObject).localStore = store
	saved := memberA.deliver(opInit, opA)
	if err := memberA.replayer.save(ctx); err != nil {
		t.Fatal(err.Error())
	}

	// A new replayer resumes it and places the next operation.
	restored := &replayTestMember{
		space: space,
		set:   memberA.set,
		snap:  memberA.snap,
		replayer: &replayer{
			c:  space.c,
			so: &testSharedObject{peerID: pidA, blockStore: space.so.blockStore, localStore: store},
		},
	}
	if err := restored.replayer.load(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if !slices.EqualFunc(restored.replayer.positions, memberA.replayer.positions, func(a, b replayPosition) bool {
		return a.state.EqualVT(b.state)
	}) {
		t.Fatal("restored positions hold Worlds other than the saved replay's")
	}
	got := restored.deliver(opB)

	// It matches a cold replay of all three.
	cold := space.member(pidA, true)
	cold.replayer.base = &InnerState{}
	want := cold.deliver(opInit, opA, opB)
	if !got.state.EqualVT(want.state) || !slices.Equal(got.outcomes, want.outcomes) {
		t.Errorf("resumed replay %q after saving %q; want %q", got.outcomes, saved.outcomes, want.outcomes)
	}
	if got.state.GetHeadRef().GetRootRef().GetEmpty() {
		t.Error("resumed replay did not apply the operation after the initialization")
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

// TestReplayKeepsPositionsAboveCheckpoint checks that a checkpoint at a
// replayed position keeps the outcomes and Worlds above it, so nothing
// replays again.
func TestReplayKeepsPositionsAboveCheckpoint(t *testing.T) {
	// A writes a chain of three operations and a member replays them.
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	a1 := space.sign("a1", privA, "object-1", &sobject.SOOperationLink{Nonce: 1})
	a2 := space.sign("a2", privA, "object-2", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	a3 := space.sign("a3", privA, "object-3", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: a2.Hash()})
	member := space.member(pidA, false)
	want := member.deliver(a1, a2, a3)

	// Checkpoint the World after a2.
	base := member.replayer.stateAfter([][]byte{a1.Hash(), a2.Hash()})
	if base == nil {
		t.Fatal("replay does not hold the World after a2")
	}
	member.replayer.base = base
	member.replayer.positions = member.replayer.positionsAbove(base)
	member.set = sobject.NewSOOperationSet(replayTestObjectID, &sobject.SOCheckpointInner{
		Authors: []*sobject.SOOperationPosition{{PeerId: pidA.String(), Nonce: 2, OpHash: a2.Hash()}},
	})

	// Resolving no config, replaying a3 again would reject it.
	member.snap = &replayTestSnapshot{config: &sobject.SharedObjectConfig{}}
	got := member.deliver(a1, a2, a3)
	if !got.state.EqualVT(want.state) || !slices.Equal(got.outcomes, want.outcomes[2:]) {
		t.Fatalf("after checkpoint %q; want %q", got.outcomes, want.outcomes[2:])
	}
}

// TestReplayDetectsWrongCheckpoint checks that a member that replayed the
// operations a checkpoint covers reports a checkpoint whose World differs from
// its own, and that a member missing a covered operation does not judge it.
func TestReplayDetectsWrongCheckpoint(t *testing.T) {
	// A writes a chain of three operations.
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	a1 := space.sign("a1", privA, "object-1", &sobject.SOOperationLink{Nonce: 1})
	a2 := space.sign("a2", privA, "object-2", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	a3 := space.sign("a3", privA, "object-3", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: a2.Hash()})

	// A checkpoint after a2 with the World after a2 agrees.
	right := space.member(pidA, false)
	right.deliver(a1, a2, a3)
	afterA1 := right.replayer.stateAfter([][]byte{a1.Hash()})
	afterA2 := right.replayer.stateAfter([][]byte{a1.Hash(), a2.Hash()})
	right.adopt(1, afterA2, a2)
	if right.replayer.mismatch != nil {
		t.Fatalf("an agreeing checkpoint reported %v", right.replayer.mismatch)
	}

	// The same checkpoint with the World after a1 is wrong.
	wrong := space.member(pidA, false)
	wrong.deliver(a1, a2, a3)
	wrong.adopt(1, afterA1, a2)
	if got := wrong.replayer.mismatch.GetHeight(); got != 1 {
		t.Fatalf("wrong checkpoint reported height %d; want 1", got)
	}

	// A later agreeing checkpoint clears the report.
	wrong.adopt(2, wrong.replayer.positions[len(wrong.replayer.positions)-1].state, a3)
	if wrong.replayer.mismatch != nil {
		t.Fatalf("an agreeing checkpoint kept %v", wrong.replayer.mismatch)
	}

	// A member holding only a1 cannot judge a checkpoint after a2.
	lagging := space.member(pidA, false)
	lagging.deliver(a1)
	lagging.adopt(1, afterA1, a2)
	if lagging.replayer.mismatch != nil {
		t.Fatalf("a member missing a covered operation reported %v", lagging.replayer.mismatch)
	}
}

// replayTestPublisher returns empty head updates.
type replayTestPublisher struct{}

// LocalStateHead returns an empty head update.
func (replayTestPublisher) LocalStateHead(string, []byte, []byte) *block.AtomicHeadUpdate {
	return &block.AtomicHeadUpdate{}
}

// AccessAdmittedState is not used by the replayer.
func (replayTestPublisher) AccessAdmittedState(context.Context, func()) (ccontainer.Watchable[sobject.SharedObjectStateSnapshot], func(), error) {
	return nil, nil, errors.New("replay test publisher has no admitted state")
}

// TestReplaySpanHoldsOnlyNewPositions checks that each save writes a span
// block holding only the Worlds the positions since the previous save reached,
// chained to the previous block, and that a new checkpoint starts a new chain.
func TestReplaySpanHoldsOnlyNewPositions(t *testing.T) {
	// A writes a chain of three operations.
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	a1 := space.sign("a1", privA, "object-1", &sobject.SOOperationLink{Nonce: 1})
	a2 := space.sign("a2", privA, "object-2", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	a3 := space.sign("a3", privA, "object-3", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: a2.Hash()})
	member := space.member(pidA, false)
	r := member.replayer

	// save publishes the pending span and returns it with its root.
	save := func() (*ReplaySpan, *block.BlockRef) {
		// Collect the pending save, which must name one new span block.
		t.Helper()
		entries, roots, _, err := r.pendingSave(replayTestPublisher{}, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(entries) != 1 || len(roots) != 1 || !roots[0].Ref.EqualsRef(entries[0].Ref) {
			t.Fatalf("save wrote %d blocks and %d roots; want one span named", len(entries), len(roots))
		}

		// Decode it and record the save.
		span := &ReplaySpan{}
		if err := span.UnmarshalVT(entries[0].Data); err != nil {
			t.Fatal(err.Error())
		}
		r.saved()
		return span, entries[0].Ref
	}

	// The first save holds both Worlds and starts the chain.
	member.deliver(a1, a2)
	first, firstRef := save()
	if len(first.GetWorlds()) != 2 || first.GetPrev() != nil {
		t.Fatalf("first span holds %d Worlds after %v; want 2 starting the chain", len(first.GetWorlds()), first.GetPrev())
	}

	// The next save holds only the World after a3, after the first block.
	member.deliver(a1, a2, a3)
	second, _ := save()
	want := r.positions[2].state.GetHeadRef().GetRootRef()
	if len(second.GetWorlds()) != 1 || !second.GetWorlds()[0].EqualsRef(want) || !second.GetPrev().EqualsRef(firstRef) {
		t.Fatalf("second span holds %d Worlds after %v; want the World after a3 after %v", len(second.GetWorlds()), second.GetPrev(), firstRef)
	}

	// A checkpoint after a2 starts a new chain holding only the World after a3.
	member.adopt(1, r.positions[1].state, a2)
	third, _ := save()
	if len(third.GetWorlds()) != 1 || !third.GetWorlds()[0].EqualsRef(want) || third.GetPrev() != nil {
		t.Fatalf("span after checkpoint holds %d Worlds after %v; want the World after a3 starting a chain", len(third.GetWorlds()), third.GetPrev())
	}
}

// TestReplayAdoptsForkAcrossCheckpoint checks that replay adopts a local write
// forked before a checkpoint rebased the replay, without applying it again.
func TestReplayAdoptsForkAcrossCheckpoint(t *testing.T) {
	// A writes a chain of three operations; another member computes the World
	// after the third.
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	a1 := space.sign("a1", privA, "object-1", &sobject.SOOperationLink{Nonce: 1})
	a2 := space.sign("a2", privA, "object-2", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	a3 := space.sign("a3", privA, "object-3", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: a2.Hash()})
	want := space.member(pidA, false).deliver(a1, a2, a3)

	// The writer replays a1 and a2 and forks a3 from the World after them.
	member := space.member(pidA, false)
	afterA2 := member.deliver(a1, a2).state
	base, head, index := member.replayer.head()
	fork := &replayFork{base: base, index: index, start: head, hash: a3.Hash(), state: want.state.CloneVT()}

	// A checkpoint at a2 rebases the replay before the write is replayed.
	member.adopt(1, afterA2, a2)
	if member.replayer.base == base || len(member.replayer.positions) != 0 {
		t.Fatal("checkpoint did not rebase the replay")
	}
	if _, err := member.set.Add(a3); err != nil {
		t.Fatal(err.Error())
	}
	state, outcomes, err := member.replayer.replay(context.Background(), member.snap, member.set, fork)
	if err != nil {
		t.Fatal(err.Error())
	}
	if state != fork.state || len(outcomes) != 1 || !bytes.Equal(outcomes[0].hash, a3.Hash()) {
		t.Fatalf("replay applied the write again instead of adopting it: %d outcomes", len(outcomes))
	}
}

// TestReplayCarriesBaseHoldUntilSaved checks that each save carries the hold
// of a checkpoint World rebase adopted until one is durable, and that a failed
// publication restores it, including when a later rebase replaced the World
// the failed publication started from.
func TestReplayCarriesBaseHoldUntilSaved(t *testing.T) {
	// A writes a chain of three operations.
	privA, pidA := newReplayTestKey(t)
	space := newReplayTestSpace(t, pidA)
	a1 := space.sign("a1", privA, "object-1", &sobject.SOOperationLink{Nonce: 1})
	a2 := space.sign("a2", privA, "object-2", &sobject.SOOperationLink{Nonce: 2, PrevOpHash: a1.Hash()})
	a3 := space.sign("a3", privA, "object-3", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: a2.Hash()})

	// The member replays them and saves its replay.
	member := space.member(pidA, false)
	r := member.replayer
	afterA2 := member.deliver(a1, a2).state
	afterA3 := member.deliver(a1, a2, a3).state
	r.saved()

	// adopt rebases onto a checkpoint and gathers a base hold naming its
	// World. The test store owns no roots, so the hold is named here.
	adopt := func(height uint64, world *InnerState, head *sobject.SOOperation) {
		// Rebase and check the hold was gathered.
		t.Helper()
		member.adopt(height, world, head)
		if r.baseHold == nil || !r.baseUnheld {
			t.Fatal("rebase did not gather the base hold")
		}

		// Name the World under the replay base root.
		ref := world.GetHeadRef().GetRootRef()
		r.baseHold = &rootHold{
			entries: []*block.PutBatchEntry{{Ref: ref}},
			roots:   []block.NamedRoot{{Name: replayBaseRootName, Ref: ref}},
		}
	}

	// carries reports whether the pending save names the base root.
	carries := func() bool {
		t.Helper()
		_, roots, _, err := r.pendingSave(replayTestPublisher{}, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		return slices.ContainsFunc(roots, func(root block.NamedRoot) bool { return root.Name == replayBaseRootName })
	}

	// The save after a rebase carries the base hold, and once it is durable
	// the next save does not.
	adopt(1, afterA2, a2)
	if !carries() {
		t.Fatal("save after a rebase does not hold the checkpoint World")
	}
	unheld, base, span, spanLen := r.baseUnheld, r.base, r.span, r.spanLen
	r.saved()
	if r.baseUnheld || carries() {
		t.Fatal("save after a durable save still holds the checkpoint World")
	}

	// A failed publication that carried it restores it.
	r.unsave(base, span, spanLen, unheld)
	if !carries() {
		t.Fatal("failed publication did not restore the base hold")
	}

	// A publication admitted with the base held, then a rebase whose hold
	// only later publications carried: when the first fails, the new base
	// is unheld again.
	r.saved()
	firstBase, firstSpan, firstLen := r.base, r.span, r.spanLen
	adopt(2, afterA3, a3)
	r.saved()
	r.unsave(firstBase, firstSpan, firstLen, false)
	if !carries() {
		t.Fatal("failed publications did not restore the hold of the later checkpoint World")
	}
}

// replayFailStore is a test block store whose writes fail with fail while it
// is set, so a fence that drains buffered blocks into it loses them.
type replayFailStore struct {
	*testBlockStore
	fail error
}

// PutBlock fails while fail is set.
func (s *replayFailStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if s.fail != nil {
		return nil, false, s.fail
	}
	return s.testBlockStore.PutBlock(ctx, data, opts)
}

// PutBlockBatch fails while fail is set.
func (s *replayFailStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if s.fail != nil {
		return nil, s.fail
	}
	return s.testBlockStore.PutBlockBatch(ctx, entries)
}

// TestReplayDropsPositionsAfterFailedBindFence fails the fence a bind runs
// mid-pass, when the World's transform changed under the engine. The blocks
// of the operation before it are lost with the engine, so replay must keep no
// position past the fenced prefix and return no World, also when the store
// reports the failure as a block not available. The next replay places the
// operations again, and a fresh engine must read both objects from the store.
func TestReplayDropsPositionsAfterFailedBindFence(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"write error", errors.New("test write failure")},
		{"unavailable", errors.Wrap(block.ErrUnavailable, "test store offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build a member over a store that can fail its writes.
			privA, pidA := newReplayTestKey(t)
			space := newReplayTestSpace(t, pidA)
			store := &replayFailStore{testBlockStore: space.so.blockStore.(*testBlockStore)}
			member := space.member(pidA, false)
			member.replayer.so = &testSharedObject{peerID: pidA, blockStore: store}

			// Create an object, move the World to s2, then create another,
			// whose bind fences the first.
			opA := space.sign("A", privA, "object-a", &sobject.SOOperationLink{Nonce: 1})
			conf, err := (&SOWorldOp{Body: &SOWorldOp_SetTransformConf{SetTransformConf: &SetTransformConfOp{
				TransformConf: newStateTestTransformConfig(t, &transform_s2.Config{ReadGzip: true}),
			}}}).MarshalVT()
			if err != nil {
				t.Fatal(err.Error())
			}
			opConf := space.signOp("s2", privA, conf, &sobject.SOOperationLink{Nonce: 2, PrevOpHash: opA.Hash()})
			opB := space.sign("B", privA, "object-b", &sobject.SOOperationLink{Nonce: 3, PrevOpHash: opConf.Hash()})
			for _, op := range []*sobject.SOOperation{opA, opConf, opB} {
				if _, err := member.set.Add(op); err != nil {
					t.Fatal(err.Error())
				}
			}

			// The bind's fence fails.
			ctx := context.Background()
			store.fail = tc.err
			state, _, err := member.replayer.replay(ctx, member.snap, member.set, nil)
			store.fail = nil
			if !errors.Is(err, tc.err) {
				t.Fatalf("replay returned %v; want the fence failure", err)
			}
			if state != nil {
				t.Fatal("replay returned a World whose blocks a failed fence lost")
			}
			if n := len(member.replayer.positions); n != 0 {
				t.Fatalf("replay kept %d positions past the fenced prefix", n)
			}

			// The next replay places all three, and their World is stored.
			got := member.deliver()
			want := []string{"A: applied", "s2: applied", "B: applied"}
			if !slices.Equal(got.outcomes, want) {
				t.Fatalf("replay outcomes %q; want %q", got.outcomes, want)
			}
			head := got.state.GetHeadRef()
			ws, err := space.c.buildBlkEngine(ctx, space.c.le, member.replayer.so, head, head.GetTransformConf())
			if err != nil {
				t.Fatalf("installed World root: %v", err)
			}
			defer ws.Release()
			read, err := ws.bengine.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer read.Discard()
			for _, key := range []string{"object-a", "object-b"} {
				objectState, found, err := read.GetObject(ctx, key)
				world.ReleaseObjectState(objectState)
				if err != nil || !found {
					t.Fatalf("installed World object %s: found %v, %v", key, found, err)
				}
			}
		})
	}
}
