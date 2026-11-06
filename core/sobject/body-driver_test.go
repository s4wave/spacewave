package sobject

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// errTestBodyBlock stands in for a block the body cannot read yet.
var errTestBodyBlock = errors.New("block not available")

// testBody is a Body whose state is the data of the applied operations joined
// in order. It refuses "solo" unless it is the first operation applied above
// the checkpoint, and fails "missing" while missing is set.
type testBody struct {
	// base is the state of the installed checkpoint.
	base string
	// states are the states after each applied operation.
	states []string
	// applies records the index of every Apply call.
	applies []int
	// installs records the covered count of every Install call.
	installs []int
	// missing makes Apply fail the "missing" operation.
	missing bool
}

// stateAt returns the state after the first n operations.
func (b *testBody) stateAt(n int) string {
	if n == 0 {
		return b.base
	}
	return b.states[n-1]
}

// Root returns the state after the first n operations.
func (b *testBody) Root(_ context.Context, n int) ([]byte, error) {
	if n > len(b.states) {
		return nil, nil
	}
	return []byte(b.stateAt(n)), nil
}

// Apply appends the operation's data to the state before index.
func (b *testBody) Apply(_ context.Context, index int, op *OrderedOperation) (string, error) {
	// Continue from the state before index.
	b.applies = append(b.applies, index)
	if index > len(b.states) {
		return "", errors.New("apply past the applied operations")
	}
	b.states = b.states[:index]
	prev := b.stateAt(index)

	// Refuse or fail by the operation's data.
	data := string(op.Data)
	reason := op.Refused
	switch {
	case data == "missing" && b.missing:
		return "", errTestBodyBlock
	case data == "solo" && prev != b.base:
		reason = "solo follows another operation"
	}
	next := prev
	if reason == "" {
		next = prev + data
	}
	b.states = append(b.states, next)
	return reason, nil
}

// Install resets the state to the checkpoint's, keeping the later states
// when the state after covered equals it.
func (b *testBody) Install(_ context.Context, checkpoint *SOCheckpointInner, covered int) (bool, error) {
	b.installs = append(b.installs, covered)
	data := string(checkpoint.GetStateData())
	kept := covered >= 0 && covered <= len(b.states) && b.stateAt(covered) == data
	if kept {
		b.states = slices.Clone(b.states[covered:])
	} else {
		b.states = nil
	}
	b.base = data
	return kept, nil
}

// testCheckpoint adopts the stable checkpoint of state covering prefix with
// the state data data, signed by the owner priv.
func testCheckpoint(t *testing.T, state *SOState, priv crypto.PrivKey, prefix [][]byte, data string) {
	t.Helper()
	xfrm, err := testHandle(t, state, priv).GetTransformer(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := xfrm.EncodeBlock([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := state.BuildStableCheckpoint(mockSharedObjectID, priv, prefix, enc)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AdoptCheckpoint(mockSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
}

// writeConcurrentOp writes data as priv's next operation naming no other
// author's operation, so it is concurrent with every operation it does not
// follow in its own chain.
func writeConcurrentOp(t *testing.T, state *SOState, priv crypto.PrivKey, data string) *SOOperation {
	// Link only to the author's own chain.
	t.Helper()
	set, err := state.OperationSet(mockSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	handle := testHandle(t, state, priv)
	link := state.NextOperationLink(set, handle.peerID.String())
	link.Parents = nil

	// Encrypt, sign and add it.
	xfrm, err := handle.GetTransformer(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	enc, err := xfrm.EncodeBlock([]byte(data))
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

// TestBodyDriverRunThrough drives a body through an order that grows, a late
// operation that sorts ahead of every applied one, a body error, a checkpoint
// the body agrees with and one it does not.
func TestBodyDriverRunThrough(t *testing.T) {
	// Hold an owner and two writers. The driver runs as the owner.
	peers := createMockPeers(t, 3)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	body := &testBody{}
	var reports []*BodyReport
	so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
	le := logrus.New().WithField("test", t.Name())
	d := NewBodyDriver(le, so, body, func(r *BodyReport) { reports = append(reports, r) })
	step := func() *OrderEvent {
		t.Helper()
		snap := testHandle(t, state, keys[0])
		if err := d.Step(t.Context(), snap); err != nil {
			t.Fatal(err)
		}
		ev, err := ReadOrder(t.Context(), snap)
		if err != nil {
			t.Fatal(err)
		}
		return ev
	}

	// The first step installs the genesis checkpoint and applies the order.
	solo := writeTestOp(t, state, keys[0], "solo")
	writeTestOp(t, state, keys[0], "a")
	step()
	if got := body.stateAt(len(body.states)); got != "soloa" {
		t.Fatalf("state %q; want soloa", got)
	}
	if !slices.Equal(body.installs, []int{-1}) || !slices.Equal(body.applies, []int{0, 1}) {
		t.Fatalf("installs %v applies %v; want [-1] and [0 1]", body.installs, body.applies)
	}

	// A step with nothing new applies nothing.
	body.applies = nil
	step()
	if len(body.applies) != 0 {
		t.Fatalf("an unchanged order applied %v", body.applies)
	}

	// A concurrent operation from another writer that sorts ahead of every
	// applied operation makes the driver apply from the first index, and the
	// owner's "solo" edit, applied before, is now refused: a revocation.
	for {
		op := writeConcurrentOp(t, state, keys[1], "b")
		if strings.Compare(string(op.Hash()), string(solo.Hash())) < 0 {
			break
		}
		state.Ops = slices.DeleteFunc(state.Ops, func(o *SOOperation) bool { return o == op })
	}
	step()
	if body.applies[0] != 0 {
		t.Fatalf("late operation applied from index %d; want 0", body.applies[0])
	}
	if got := body.stateAt(len(body.states)); got != "ba" {
		t.Fatalf("state %q; want ba", got)
	}
	last := reports[len(reports)-1]
	if len(last.RejectedEdits) != 1 || string(last.RejectedEdits[0].GetOpHash()) != string(solo.Hash()) {
		t.Fatalf("rejected edits %v; want the solo edit", last.RejectedEdits)
	}
	if !slices.Equal(last.RejectedEdits[0].GetLostToPeerIds(), []string{peers[1].GetPeerID().String()}) {
		t.Fatalf("solo lost to %v; want the late writer", last.RejectedEdits[0].GetLostToPeerIds())
	}

	// A body error stops the step at its operation, and the next step
	// resumes there.
	body.missing = true
	writeTestOp(t, state, keys[2], "missing")
	snap := testHandle(t, state, keys[0])
	if err := d.Step(t.Context(), snap); !errors.Is(err, errTestBodyBlock) {
		t.Fatalf("step error %v; want the missing block", err)
	}
	body.missing = false
	body.applies = nil
	ev := step()
	if !slices.Equal(body.applies, []int{len(ev.Order) - 1}) {
		t.Fatalf("resumed at %v; want [%d]", body.applies, len(ev.Order)-1)
	}

	// A checkpoint over the first two operations with the state the body
	// reached there keeps the operations after them.
	prefix := ev.Order[:2]
	testCheckpoint(t, state, keys[0], prefix, body.stateAt(2))
	body.applies = nil
	step()
	if !slices.Equal(body.installs[len(body.installs)-1:], []int{2}) || len(body.applies) != 0 {
		t.Fatalf("install %v applies %v; want covered 2 and nothing applied", body.installs, body.applies)
	}
	if last := reports[len(reports)-1]; last.CheckpointMismatch != nil {
		t.Fatalf("matching checkpoint reported a mismatch at %d", last.CheckpointMismatch.GetHeight())
	}

	// A checkpoint with another state is a mismatch: the body applies every
	// operation above it again.
	writeTestOp(t, state, keys[0], "c")
	ev = step()
	testCheckpoint(t, state, keys[0], ev.Order[:1], "other")
	body.applies = nil
	ev = step()
	if last := reports[len(reports)-1]; last.CheckpointMismatch == nil {
		t.Fatal("differing checkpoint reported no mismatch")
	}
	if got := body.stateAt(len(body.states)); len(body.applies) != len(ev.Order) || !strings.HasPrefix(got, "other") || !strings.HasSuffix(got, "c") {
		t.Fatalf("applies %v state %q; want every operation over other", body.applies, body.stateAt(len(body.states)))
	}
}

// TestBodyDriverCheckpoints checks that an owner's driver signs a checkpoint
// over the stable prefix once MinCheckpointOperations operations are stable,
// with the state the body's Root holds, and that the next step installs it
// keeping nothing to apply again.
func TestBodyDriverCheckpoints(t *testing.T) {
	// Host a Space whose only writer is the owner.
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	g := newGroupHost(t.Context(), state)
	body := &testBody{}
	so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
	le := logrus.New().WithField("test", t.Name())
	d := NewBodyDriver(le, so, body, nil)
	d.SetCheckpointer(g.host, keys[0], testStepFactorySet())

	// Below the threshold the driver signs nothing.
	held := g.ctr.GetValue().Clone()
	for range MinCheckpointOperations - 1 {
		writeTestOp(t, held, keys[0], "x")
	}
	g.ctr.SetValue(held)
	if err := d.Step(t.Context(), testHandle(t, g.ctr.GetValue(), keys[0])); err != nil {
		t.Fatal(err)
	}
	if h := mustHeight(t, g.ctr.GetValue()); h != 0 {
		t.Fatalf("checkpointed below the threshold at height %d", h)
	}

	// At the threshold it signs one over every stable operation, carrying
	// the body's state there.
	held = g.ctr.GetValue().Clone()
	writeTestOp(t, held, keys[0], "y")
	g.ctr.SetValue(held)
	if err := d.Step(t.Context(), testHandle(t, g.ctr.GetValue(), keys[0])); err != nil {
		t.Fatal(err)
	}
	if h := mustHeight(t, g.ctr.GetValue()); h != 1 {
		t.Fatalf("checkpoint height %d; want 1", h)
	}
	if n := len(g.ctr.GetValue().GetOps()); n != 0 {
		t.Fatalf("the checkpoint left %d operations", n)
	}

	// The next step installs it: the body reached its state, so nothing is
	// applied again.
	body.applies = nil
	if err := d.Step(t.Context(), testHandle(t, g.ctr.GetValue(), keys[0])); err != nil {
		t.Fatal(err)
	}
	if got := body.installs[len(body.installs)-1]; got != MinCheckpointOperations {
		t.Fatalf("installed covering %d; want %d", got, MinCheckpointOperations)
	}
	if len(body.applies) != 0 || body.base != strings.Repeat("x", MinCheckpointOperations-1)+"y" {
		t.Fatalf("applies %v base %q after the checkpoint", body.applies, body.base)
	}
}

// mustHeight returns the height of state's checkpoint.
func mustHeight(t *testing.T, state *SOState) uint64 {
	t.Helper()
	inner, err := state.GetCheckpointInner()
	if err != nil {
		t.Fatal(err)
	}
	return inner.GetHeight()
}

// TestBodyHostAttachesOneBody checks that a body host attaches one body at a
// time: a second body waits until the first is released, and a wait whose
// context ends attaches nothing.
func TestBodyHostAttachesOneBody(t *testing.T) {
	// Attach a first body.
	h := NewBodyHost(logrus.NewEntry(logrus.New()), nil, nil)
	first, second := &testBody{}, &testBody{}
	release, err := h.SetBody(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}

	// A second attach whose context ends while the first is attached fails.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.SetBody(ctx, second); !errors.Is(err, context.Canceled) {
		t.Fatalf("attach beside an attached body = %v, want canceled", err)
	}

	// A second attach waits for the first's release, then attaches.
	attached := make(chan func(), 1)
	go func() {
		rel, err := h.SetBody(t.Context(), second)
		if err != nil {
			t.Error(err)
		}
		attached <- rel
	}()
	select {
	case <-attached:
		t.Fatal("a second body attached while the first was attached")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	(<-attached)()
}
