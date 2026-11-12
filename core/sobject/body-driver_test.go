package sobject

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/block"
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
	// storeGate, when set, holds each store Root returns until it closes.
	storeGate chan struct{}
	// storeErr, when set, is the result of each store Root returns.
	storeErr func() error
	// applyErr, when set, is the result of each Apply of the "refused"
	// operation that it does not answer nil.
	applyErr func() error
}

// stateAt returns the state after the first n operations.
func (b *testBody) stateAt(n int) string {
	if n == 0 {
		return b.base
	}
	return b.states[n-1]
}

// Root returns the state after the first n operations.
func (b *testBody) Root(_ context.Context, n int) ([]byte, func(context.Context) error, error) {
	if n > len(b.states) {
		return nil, nil, nil
	}
	gate, storeErr := b.storeGate, b.storeErr
	return []byte(b.stateAt(n)), func(ctx context.Context) error {
		if storeErr != nil {
			return storeErr()
		}
		if gate == nil {
			return nil
		}
		select {
		case <-gate:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}, nil
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
	case data == "refused" && b.applyErr != nil:
		if err := b.applyErr(); err != nil {
			return "", err
		}
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

	// Over 10 and 100 operations above the checkpoint, a step after one new
	// operation applies only it and installs nothing, as the Phase 3 path
	// walk says: the apply loop is proportional to the new positions.
	for _, n := range []int{10, 100} {
		state, _ := newTestSOState(t, peers)
		body := &testBody{}
		d := NewBodyDriver(le, so, body, nil)
		for i := range n {
			writeTestOp(t, state, keys[i%len(keys)], "x")
		}
		if err := d.Step(t.Context(), testHandle(t, state, keys[0])); err != nil {
			t.Fatal(err)
		}
		body.applies, body.installs = nil, nil
		writeTestOp(t, state, keys[0], "y")
		if err := d.Step(t.Context(), testHandle(t, state, keys[0])); err != nil {
			t.Fatal(err)
		}
		if len(body.applies) != 1 || len(body.installs) != 0 {
			t.Fatalf("over %d operations a new one applied %v and installed %v; want one apply", n, body.applies, body.installs)
		}
	}
}

// TestBodyDriverDropsOperationsACutRemoves checks that a driver whose body
// applied a writer's operations, then sees a removal that cuts the writer
// through its first one, applies again from the first cut operation and
// reaches the state of a driver that starts after the cut. It fails when the
// driver keeps every applied position the order still lists.
func TestBodyDriverDropsOperationsACutRemoves(t *testing.T) {
	// The writer writes two operations and the owner one.
	peers := createMockPeers(t, 2)
	keys := mustPrivKeys(t, peers)
	removedID := peers[1].GetPeerID().String()
	state, genesis := newTestSOState(t, peers)
	writeTestOp(t, state, keys[1], "w1")
	writeTestOp(t, state, keys[1], "w2")
	writeTestOp(t, state, keys[0], "a")

	// The warm driver applies all three.
	so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
	le := logrus.New().WithField("test", t.Name())
	warm := &testBody{}
	dw := NewBodyDriver(le, so, warm, nil)
	if err := dw.Step(t.Context(), testHandle(t, state, keys[0], genesis)); err != nil {
		t.Fatal(err)
	}
	if got := warm.stateAt(len(warm.states)); !strings.Contains(got, "w2") {
		t.Fatalf("state %q before the cut; want w2 applied", got)
	}

	// The owner removes the writer through its first operation.
	next := state.GetConfig().CloneVT()
	next.Participants = next.Participants[:1]
	if err := pinRemovedAuthors(mockSharedObjectID, state, next, []string{removedID}, map[string]uint64{removedID: 1}); err != nil {
		t.Fatal(err)
	}
	removal, err := BuildSOConfigChange(mockSharedObjectID, state.GetConfig(), next, SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT, keys[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config, err = VerifyConfigChange(mockSharedObjectID, state.GetConfig(), removal)
	if err != nil {
		t.Fatal(err)
	}

	// Both drivers step under the cut and reach the same state, without w2.
	cold := &testBody{}
	dc := NewBodyDriver(le, so, cold, nil)
	for _, d := range []*BodyDriver{dw, dc} {
		if err := d.Step(t.Context(), testHandle(t, state, keys[0], genesis)); err != nil {
			t.Fatal(err)
		}
	}
	got, want := warm.stateAt(len(warm.states)), cold.stateAt(len(cold.states))
	if got != want || strings.Contains(got, "w2") || !strings.Contains(got, "w1") {
		t.Fatalf("after the cut the warm state is %q and the cold one %q; want them equal, with w1 and without w2", got, want)
	}
}

// TestBodyDriverCheckpoints checks that an owner's driver signs a checkpoint
// over the stable prefix once MinCheckpointOperations operations are stable,
// with the state the body's Root holds, once the store Root returns finished
// in the background while the driver applied a later operation, and that the
// next step installs it keeping nothing to apply again.
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

	// At the threshold it takes the body's state there and stores it in the
	// background: while the store runs the driver applies a later operation
	// and signs nothing, and once the store returns it signs a checkpoint
	// over every stable operation it took, carrying that state.
	body.storeGate = make(chan struct{})
	held = g.ctr.GetValue().Clone()
	writeTestOp(t, held, keys[0], "y")
	g.ctr.SetValue(held)
	if err := d.Step(t.Context(), testHandle(t, g.ctr.GetValue(), keys[0])); err != nil {
		t.Fatal(err)
	}
	held = g.ctr.GetValue().Clone()
	writeTestOp(t, held, keys[0], "z")
	g.ctr.SetValue(held)
	if err := d.Step(t.Context(), testHandle(t, g.ctr.GetValue(), keys[0])); err != nil {
		t.Fatal(err)
	}
	if n := len(body.states); n != MinCheckpointOperations+1 {
		t.Fatalf("applied %d operations while the checkpoint stored; want %d", n, MinCheckpointOperations+1)
	}
	if h := mustHeight(t, g.ctr.GetValue()); h != 0 {
		t.Fatalf("checkpointed at height %d before its store returned", h)
	}
	close(body.storeGate)
	if _, err := g.ctr.WaitValueWithValidator(t.Context(), func(state *SOState) (bool, error) {
		return mustHeight(t, state) == 1, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(g.ctr.GetValue().GetOps()); n != 1 {
		t.Fatalf("the checkpoint left %d operations; want the later one", n)
	}

	// The next step installs it: the body reached its state, so it keeps the
	// later operation and applies nothing again.
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

// TestBodyDriverDamagedByRefusedStore checks that a checkpoint store the body
// refuses with a block.MissingChildError is tried once more, then, refused again,
// damages the body: the driver stores no further checkpoint until the
// refusal is due again, so it does not spin, and the report shows the
// refusal. Once the cause heals, the store due after the retry delay signs
// the checkpoint and clears the damage. A store that heals on the second
// attempt signs the checkpoint and shows no damage.
func TestBodyDriverDamagedByRefusedStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		// refusals is how many stores the body refuses, or -1 for all.
		refusals int
	}{
		{name: "forever", refusals: -1},
		{name: "healed", refusals: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Host a Space whose only writer is the owner, whose body refuses
			// its stores.
			ctx := t.Context()
			peers := createMockPeers(t, 1)
			keys := mustPrivKeys(t, peers)
			state, _ := newTestSOState(t, peers)
			g := newGroupHost(ctx, state)
			refused := fmt.Errorf("store checkpoint: %w", &block.MissingChildError{Op: "Flush", Block: &block.BlockRef{}, Child: &block.BlockRef{}})
			stores, healed := 0, false
			body := &testBody{storeErr: func() error {
				stores++
				if !healed && (tc.refusals < 0 || stores <= tc.refusals) {
					return refused
				}
				return nil
			}}
			var reports []*BodyReport
			so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
			d := NewBodyDriver(logrus.New().WithField("test", t.Name()), so, body, func(r *BodyReport) { reports = append(reports, r) })
			d.SetCheckpointer(g.host, keys[0], testStepFactorySet())

			// Reach the threshold, then step on every wake a store sends
			// until none arrives.
			held := g.ctr.GetValue().Clone()
			for range MinCheckpointOperations {
				writeTestOp(t, held, keys[0], "x")
			}
			g.ctr.SetValue(held)
			settle := func() {
				t.Helper()
				for range 10 {
					if err := d.Step(ctx, testHandle(t, g.ctr.GetValue(), keys[0])); err != nil {
						t.Fatal(err)
					}
					select {
					case <-d.wake:
						continue
					case <-time.After(200 * time.Millisecond):
						return
					}
				}
				t.Fatalf("the driver kept storing: %d stores", stores)
			}
			settle()

			// A refusal that repeats damages the body after two stores; a
			// healed one signs the checkpoint.
			damage := reports[len(reports)-1].Damage
			if tc.refusals < 0 {
				if stores != 2 || d.Damaged().GetOperation() != "Flush" || d.Damaged().GetRefusals() != 2 || !damage.EqualVT(d.Damaged()) {
					t.Fatalf("stores %d, damage %v, reported %v; want 2 and the refusal", stores, d.Damaged(), damage)
				}

				// The cause heals; the store due after the retry delay
				// signs the checkpoint and clears the damage.
				healed = true
				d.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
				settle()
				damage = reports[len(reports)-1].Damage
			}
			if d.Damaged() != nil || damage != nil || mustHeight(t, g.ctr.GetValue()) != 1 {
				t.Fatalf("damage %v, reported %v, height %d after a healed store", d.Damaged(), damage, mustHeight(t, g.ctr.GetValue()))
			}
		})
	}
}

// execTestObject is the shared object of a driver under Execute.
type execTestObject struct {
	testDriverSharedObject
	// ctr holds the state Execute watches.
	ctr *ccontainer.CContainer[SharedObjectStateSnapshot]
}

// AccessSharedObjectState returns the state container.
func (o *execTestObject) AccessSharedObjectState(context.Context, func()) (ccontainer.Watchable[SharedObjectStateSnapshot], func(), error) {
	return o.ctr, func() {}, nil
}

// TestBodyDriverDamagedStepRetries checks that Execute steps a damaged body
// again after the damage retry delay with no new state, and not sooner at the
// failure backoff, so a Space that sends nothing more clears the damage once
// its cause heals and a refusal that stays costs one step per delay. It fails
// when a damaged step waits only for the backoff.
func TestBodyDriverDamagedStepRetries(t *testing.T) {
	// A quiet Space holds one operation the body refuses until healed.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	writeTestOp(t, state, keys[0], "refused")
	var healed atomic.Bool
	var attempts atomic.Int32
	body := &testBody{applyErr: func() error {
		attempts.Add(1)
		if healed.Load() {
			return nil
		}
		return &block.MissingChildError{Op: "Flush", Block: &block.BlockRef{}, Child: &block.BlockRef{}}
	}}
	so := &execTestObject{
		testDriverSharedObject: testDriverSharedObject{peerID: peers[0].GetPeerID()},
		ctr:                    ccontainer.NewCContainer[SharedObjectStateSnapshot](testHandle(t, state, keys[0])),
	}
	d := NewBodyDriver(logrus.New().WithField("test", t.Name()), so, body, nil)
	d.damageRetry = 1500 * time.Millisecond
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Execute(ctx) }()
	defer func() {
		cancel()
		<-done
	}()

	// The refusal repeats and damages the body.
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}
	waitFor("the damage", func() bool { return d.Damaged() != nil })

	// The damaged step waits for the damage retry delay, past the backoff.
	damaged := attempts.Load()
	time.Sleep(time.Second)
	if n := attempts.Load() - damaged; n != 0 {
		t.Fatalf("a damaged step stepped %d more times within a second", n)
	}

	// The cause heals with no new state; the retry clears the damage.
	healed.Store(true)
	waitFor("the healed step", func() bool { return d.Damaged() == nil })
}

// damagedTestBody is a testBody whose own writes are damaged until a Retry
// after heal, whose write lands in the background as a cursor save does.
type damagedTestBody struct {
	*testBody
	// healed makes the next Retry clear the damage.
	healed atomic.Bool
	// damaged reports the damage.
	damaged atomic.Bool
	// holds keeps a healed write in flight: it never lands.
	holds atomic.Bool
	// retries counts the Retry calls.
	retries atomic.Int32
	// due is when the healed write in flight was due, in Unix nanoseconds.
	due atomic.Int64
	// wake is the driver's wake.
	wake func()
}

// Damaged returns a damage while damaged is set.
func (b *damagedTestBody) Damaged() *SOStoreDamage {
	if !b.damaged.Load() {
		return nil
	}
	return &SOStoreDamage{Operation: "Flush", Refusals: 2}
}

// RetryAt returns a retry 50ms ahead while damaged, and once healed the time
// the write in flight was due, which has passed, as a body whose refusal
// stays due while its write runs: only the wake that write sends when it
// lands publishes the heal.
func (b *damagedTestBody) RetryAt() time.Time {
	switch {
	case !b.damaged.Load():
		return time.Time{}
	case b.healed.Load() && b.due.Load() != 0:
		return time.Unix(0, b.due.Load())
	default:
		return time.Now().Add(50 * time.Millisecond)
	}
}

// Retry counts the call, and once healed clears the damage after the call
// returns, as a background write landing does, then wakes the driver.
func (b *damagedTestBody) Retry(context.Context) error {
	b.retries.Add(1)
	if b.healed.Load() && b.due.CompareAndSwap(0, time.Now().UnixNano()) && !b.holds.Load() {
		go func() {
			time.Sleep(20 * time.Millisecond)
			b.damaged.Store(false)
			if b.wake != nil {
				b.wake()
			}
		}()
	}
	return nil
}

// WatchDamage keeps the driver's wake.
func (b *damagedTestBody) WatchDamage(wake func()) {
	b.wake = wake
}

// TestBodyDriverRetriesDamagedBody checks that Execute has a body whose own
// writes are damaged write them again when the body's retry is due on a
// quiet Space whose steps all succeed, and that the published health clears
// once the healed write lands in the background, with no new state. It fails
// when only a failed step wakes the driver, or when the landed write does not
// wake it to publish.
func TestBodyDriverRetriesDamagedBody(t *testing.T) {
	// A quiet Space holds one operation the body applies.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	writeTestOp(t, state, keys[0], "op")
	body := &damagedTestBody{testBody: &testBody{}}
	body.damaged.Store(true)
	so := &execTestObject{
		testDriverSharedObject: testDriverSharedObject{peerID: peers[0].GetPeerID()},
		ctr:                    ccontainer.NewCContainer[SharedObjectStateSnapshot](testHandle(t, state, keys[0])),
	}
	var shown atomic.Pointer[BodyReport]
	d := NewBodyDriver(logrus.New().WithField("test", t.Name()), so, body, func(r *BodyReport) { shown.Store(r) })
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Execute(ctx) }()
	defer func() {
		cancel()
		<-done
	}()

	// The first step retries the damaged write, which stays refused.
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}
	waitFor("the first retry", func() bool { return body.retries.Load() != 0 })
	waitFor("the damage in the health", func() bool { r := shown.Load(); return r != nil && r.Damage != nil })

	// The cause heals with no new state; a later retry's write lands after
	// it returns, and the health clears.
	body.healed.Store(true)
	waitFor("the healed health", func() bool { r := shown.Load(); return r != nil && r.Damage == nil })
}

// TestBodyDriverBacksOffDueRetryInFlight checks that Execute does not spin
// when a body's damaged write stays due while its retry runs in the
// background: the driver wakes on its backoff until the write lands. It
// fails when the driver wakes at once for a due write the step left due.
func TestBodyDriverBacksOffDueRetryInFlight(t *testing.T) {
	// A quiet Space holds one operation; the body's healed write never lands.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	writeTestOp(t, state, keys[0], "op")
	body := &damagedTestBody{testBody: &testBody{}}
	body.damaged.Store(true)
	body.healed.Store(true)
	body.holds.Store(true)
	so := &execTestObject{
		testDriverSharedObject: testDriverSharedObject{peerID: peers[0].GetPeerID()},
		ctr:                    ccontainer.NewCContainer[SharedObjectStateSnapshot](testHandle(t, state, keys[0])),
	}
	d := NewBodyDriver(logrus.New().WithField("test", t.Name()), so, body, nil)
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Execute(ctx) }()

	// Within a second the backoff allows the first step and a few more.
	time.Sleep(time.Second)
	cancel()
	<-done
	if n := body.retries.Load(); n == 0 || n > 5 {
		t.Fatalf("the driver stepped %d times in a second on a due write in flight", n)
	}
}

// TestBodyHostVoterRetriesRefusedStore checks, through the BodyHost that
// wires a group voter's Control to its driver, that a voter whose checkpoint
// store is refused on a quiet Space proposes again on its own: once each
// time the refusal comes due, on a backoff while the store fails fast with
// another error, and once healed the damage clears. It fails when a due
// refusal wakes no vote, or a store failing fast while due wakes the voter
// at once.
func TestBodyHostVoterRetriesRefusedStore(t *testing.T) {
	// Host a quiet Space under group control with enough stable operations
	// to propose, whose body refuses every store until told otherwise.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state := newGroupState(t, peers, 1)
	for range MinCheckpointOperations {
		writeTestOp(t, state, keys[0], "x")
	}
	g := newGroupHost(ctx, state)
	const (
		refuse = iota
		fail
		heal
	)
	var mode, stores atomic.Int32
	body := &testBody{storeErr: func() error {
		stores.Add(1)
		switch mode.Load() {
		case refuse:
			return &block.MissingChildError{Op: "Flush", Block: &block.BlockRef{}, Child: &block.BlockRef{}}
		case fail:
			return errors.New("store unreachable")
		}
		return nil
	}}
	so := &execTestObject{
		testDriverSharedObject: testDriverSharedObject{peerID: peers[0].GetPeerID()},
		ctr:                    ccontainer.NewCContainer[SharedObjectStateSnapshot](testHandle(t, state, keys[0])),
	}
	var shown atomic.Pointer[BodyReport]
	h := NewBodyHost(logrus.New().WithField("test", t.Name()), so, func(r *BodyReport) { shown.Store(r) })
	h.damageRetry = 200 * time.Millisecond
	h.SetSigner(g.host, keys[0], testStepFactorySet())
	h.SetContext(ctx)
	defer h.ClearContext()
	release, err := h.SetBody(ctx, body)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !cond(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}
	countFor := func(d time.Duration) int32 {
		n := stores.Load()
		time.Sleep(d)
		return stores.Load() - n
	}

	// Two refused proposals damage the voter; with no new state, the voter
	// proposes again each time the refusal comes due.
	waitFor("the damage", func() bool { r := shown.Load(); return r != nil && r.Damage != nil })
	if n := countFor(2 * time.Second); n < 2 || n > 16 {
		t.Fatalf("a due refusal stored %d times in 2s, want about one per 200ms", n)
	}

	// A store that fails fast while due retries on the backoff, not at once.
	mode.Store(fail)
	if n := countFor(1500 * time.Millisecond); n < 1 || n > 5 {
		t.Fatalf("a due store failing fast stored %d times in 1.5s, want a backoff", n)
	}

	// The cause heals; the next due vote stores and clears the damage.
	mode.Store(heal)
	waitFor("the healed health", func() bool { r := shown.Load(); return r != nil && r.Damage == nil })
}

// TestBodyHostVoterRetriesFailedStore checks, through the BodyHost that wires
// a group voter's Control to its driver, that a voter whose checkpoint store
// fails with an error that is not a refusal, with no refusal held, proposes
// again on its own on a quiet Space: on the store's backoff while it fails,
// and the group decides the checkpoint once it heals. It fails when the
// failed store arms no wake, or retries it at once.
func TestBodyHostVoterRetriesFailedStore(t *testing.T) {
	// Host a quiet single-voter Space with enough stable operations to
	// propose, whose store fails fast until healed.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state := newGroupState(t, peers, 1)
	for range MinCheckpointOperations {
		writeTestOp(t, state, keys[0], "x")
	}
	g := newGroupHost(ctx, state)
	var healed atomic.Bool
	var stores atomic.Int32
	body := &testBody{storeErr: func() error {
		stores.Add(1)
		if healed.Load() {
			return nil
		}
		return errors.New("store unreachable")
	}}
	so := &execTestObject{
		testDriverSharedObject: testDriverSharedObject{peerID: peers[0].GetPeerID()},
		ctr:                    ccontainer.NewCContainer[SharedObjectStateSnapshot](testHandle(t, state, keys[0])),
	}
	h := NewBodyHost(logrus.New().WithField("test", t.Name()), so, nil)
	h.SetSigner(g.host, keys[0], testStepFactorySet())
	h.SetContext(ctx)
	defer h.ClearContext()
	release, err := h.SetBody(ctx, body)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The store fails at 0, then retries after 250ms, 500ms and 1s.
	time.Sleep(2 * time.Second)
	if n := stores.Load(); n < 2 || n > 6 {
		t.Fatalf("a failing vote store stored %d times in 2s, want a backoff", n)
	}

	// Healed, the next retry stores and the group decides the checkpoint.
	healed.Store(true)
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := g.ctr.WaitValueWithValidator(wctx, func(state *SOState) (bool, error) {
		return mustHeight(t, state) >= 1, nil
	}, nil); err != nil {
		t.Fatalf("no checkpoint decided after the store healed: %v", err)
	}
}

// TestBodyDriverVoterDropsUnowedRefusal checks that a group voter's refused
// store clears once the voter has nothing to vote on, so a store no one
// will retry does not stay in the health. It fails when the refusal outlives
// what it was owed for.
func TestBodyDriverVoterDropsUnowedRefusal(t *testing.T) {
	// Damage a voter with two refused proposals.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state := newGroupState(t, peers, 1)
	g := newGroupHost(ctx, state)
	body := &testBody{storeErr: func() error {
		return &block.MissingChildError{Op: "Flush", Block: &block.BlockRef{}, Child: &block.BlockRef{}}
	}}
	so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
	d := NewBodyDriver(logrus.New().WithField("test", t.Name()), so, body, nil)
	d.SetCheckpointer(g.host, keys[0], testStepFactorySet())
	held := g.ctr.GetValue().Clone()
	for range MinCheckpointOperations {
		writeTestOp(t, held, keys[0], "x")
	}
	if err := d.Step(ctx, testHandle(t, held, keys[0])); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := d.ProposeCheckpoint(ctx, held); !errors.Is(err, block.ErrMissingChild) {
			t.Fatalf("proposal = %v, want a refusal", err)
		}
	}
	if d.Damaged() == nil {
		t.Fatal("two refused proposals left no damage")
	}

	// A state with no stable operation leaves nothing to vote on.
	if err := d.Step(ctx, testHandle(t, state, keys[0])); err != nil {
		t.Fatal(err)
	}
	if damage := d.Damaged(); damage != nil {
		t.Fatalf("damage %v outlived the vote it was owed for", damage)
	}
}

// TestBodyDriverOwnerStoreBacksOff checks that the owner's checkpoint store,
// failing fast with an error that is not a refusal, tries again after a
// backoff rather than on the wake its failure sends. It fails when a failed
// store wakes the driver to store again at once.
func TestBodyDriverOwnerStoreBacksOff(t *testing.T) {
	// Host a Space whose only writer is the owner, whose store fails fast.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state, _ := newTestSOState(t, peers)
	g := newGroupHost(ctx, state)
	var stores atomic.Int32
	body := &testBody{storeErr: func() error {
		stores.Add(1)
		return errors.New("store unreachable")
	}}
	so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
	d := NewBodyDriver(logrus.New().WithField("test", t.Name()), so, body, nil)
	d.SetCheckpointer(g.host, keys[0], testStepFactorySet())
	held := g.ctr.GetValue().Clone()
	for range MinCheckpointOperations {
		writeTestOp(t, held, keys[0], "x")
	}
	g.ctr.SetValue(held)

	// Step on every wake that comes within 200ms, before the first 250ms
	// backoff ends: one store, then none.
	step := func() {
		t.Helper()
		if err := d.Step(ctx, testHandle(t, held, keys[0])); err != nil {
			t.Fatal(err)
		}
	}
	step()
	for range 10 {
		select {
		case <-d.wake:
			step()
			continue
		case <-time.After(200 * time.Millisecond):
		}
		break
	}
	if n := stores.Load(); n != 1 {
		t.Fatalf("a failing store stored %d times before its backoff", n)
	}

	// The backoff wakes the driver, and the store runs again.
	select {
	case <-d.wake:
	case <-time.After(5 * time.Second):
		t.Fatal("the backoff woke no step")
	}
	step()
	deadline := time.Now().Add(5 * time.Second)
	for stores.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := stores.Load(); n != 2 {
		t.Fatalf("stores %d after the backoff, want 2", n)
	}
}

// TestBodyDriverVoterDamagedByRefusedStore checks that a group voter's
// proposal counts a refused checkpoint store as a signed checkpoint does: the
// proposal fails while the refusal is new, and once it repeated the driver
// proposes nothing and stores no more until the refusal is due again, so a
// voter neither spins on nor signs a checkpoint over the refused store. Once
// the cause heals, the proposal due after the retry delay clears the damage.
func TestBodyDriverVoterDamagedByRefusedStore(t *testing.T) {
	// Host a Space under group control whose body refuses every store.
	ctx := t.Context()
	peers := createMockPeers(t, 1)
	keys := mustPrivKeys(t, peers)
	state := newGroupState(t, peers, 1)
	g := newGroupHost(ctx, state)
	stores, healed := 0, false
	var d *BodyDriver
	body := &testBody{storeErr: func() error {
		// The store runs without the body's lock, as the signer's does.
		if !d.mtx.TryLock() {
			t.Error("a vote's store ran under the body's lock")
		} else {
			d.mtx.Unlock()
		}
		stores++
		if healed {
			return nil
		}
		return &block.MissingChildError{Op: "Flush", Block: &block.BlockRef{}, Child: &block.BlockRef{}}
	}}
	so := &testDriverSharedObject{peerID: peers[0].GetPeerID()}
	d = NewBodyDriver(logrus.New().WithField("test", t.Name()), so, body, nil)
	woken := func() bool {
		select {
		case <-d.wake:
			return true
		default:
			return false
		}
	}
	d.SetCheckpointer(g.host, keys[0], testStepFactorySet())

	// Apply enough stable operations to propose a checkpoint.
	held := g.ctr.GetValue().Clone()
	for range MinCheckpointOperations {
		writeTestOp(t, held, keys[0], "x")
	}
	g.ctr.SetValue(held)
	if err := d.Step(ctx, testHandle(t, held, keys[0])); err != nil {
		t.Fatal(err)
	}
	if stores != 0 {
		t.Fatalf("a group voter's step stored %d checkpoints", stores)
	}

	// Each vote asks for a proposal; two refusals damage the body.
	for i := range 5 {
		inner, err := d.ProposeCheckpoint(ctx, held)
		refused := errors.Is(err, block.ErrMissingChild)
		if inner != nil || refused != (i < 2) || (!refused && err != nil) {
			t.Fatalf("proposal %d = %v, %v", i, inner, err)
		}
	}
	if stores != 2 || d.Damaged().GetRefusals() != 2 {
		t.Fatalf("stores %d, damage %v; want 2 stores and the refusal", stores, d.Damaged())
	}
	if !woken() {
		t.Fatal("the damage woke no step to publish the health")
	}

	// The cause heals; the proposal due after the retry delay stores and
	// clears the damage, and wakes a step to publish it.
	healed = true
	d.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	inner, err := d.ProposeCheckpoint(ctx, held)
	if err != nil || inner == nil || d.Damaged() != nil {
		t.Fatalf("proposal after the heal = %v, %v; damage %v", inner, err, d.Damaged())
	}
	if !woken() {
		t.Fatal("the heal woke no step to publish the health")
	}

	// Judging that proposal over a refusing store counts the same way: two
	// refusals damage the body, and the next judgment stores nothing.
	healed, stores = false, 0
	for i := range 3 {
		ok, decided, err := d.JudgeCheckpoint(ctx, held, inner)
		refused := errors.Is(err, block.ErrMissingChild)
		if ok || decided || refused != (i < 2) || (!refused && err != nil) {
			t.Fatalf("judgment %d = %v, %v, %v", i, ok, decided, err)
		}
	}
	if stores != 2 || d.Damaged().GetRefusals() != 2 {
		t.Fatalf("judging: stores %d, damage %v; want 2 stores and the refusal", stores, d.Damaged())
	}

	// Healed and due again, the judgment accepts and clears the damage.
	healed = true
	d.now = func() time.Time { return time.Now().Add(4 * time.Minute) }
	ok, decided, err := d.JudgeCheckpoint(ctx, held, inner)
	if !ok || !decided || err != nil || d.Damaged() != nil {
		t.Fatalf("judgment after the heal = %v, %v, %v; damage %v", ok, decided, err, d.Damaged())
	}
	if !woken() {
		t.Fatal("the judgment's heal woke no step to publish the health")
	}
}
