package space_world_test

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// heldOps resolves the mock operations, and holds a replayed run while armed
// until it is released, so a test can act while the driver steps another
// device's operation.
type heldOps struct {
	mtx sync.Mutex
	// release is closed to let the held run go on, or nil while not armed.
	release chan struct{}
	// entered receives once for each held run as it starts.
	entered chan struct{}
}

// newHeldOps returns the operations, not armed.
func newHeldOps() *heldOps {
	return &heldOps{entered: make(chan struct{}, 16)}
}

// arm holds the next replayed runs until the returned release is called.
func (h *heldOps) arm() func() {
	ch := make(chan struct{})
	h.mtx.Lock()
	h.release = ch
	h.mtx.Unlock()
	return func() {
		h.mtx.Lock()
		h.release = nil
		h.mtx.Unlock()
		close(ch)
	}
}

// lookup resolves a mock operation that holds its replayed runs while armed.
func (h *heldOps) lookup(ctx context.Context, opTypeID string) (world.Operation, error) {
	op, err := world_mock.LookupMockOp(ctx, opTypeID)
	if op == nil || err != nil {
		return op, err
	}
	return &heldOp{Operation: op, h: h}, nil
}

// heldOp is a mock operation whose replayed runs heldOps may hold.
type heldOp struct {
	world.Operation
	h *heldOps
}

// ApplyWorldOp waits, on a replayed run while armed, until released, then
// applies the operation.
func (o *heldOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	o.h.mtx.Lock()
	release := o.h.release
	o.h.mtx.Unlock()
	if release != nil && space_world.IsReplay(ctx) {
		o.h.entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	return o.Operation.ApplyWorldOp(ctx, le, ws, sender)
}

// mockOp encodes the mock World operation on key from sender.
func mockOp(t *testing.T, key, msg string, sender peer.ID) []byte {
	t.Helper()
	tx, err := world_block_tx.NewTxApplyWorldOp(world_mock.NewMockWorldOp(key, msg), sender)
	if err != nil {
		t.Fatal(err)
	}
	data, err := (&space_world.WorldOp{Body: &space_world.WorldOp_ApplyTxOp{ApplyTxOp: &space_world.ApplyTxOp{Tx: tx}}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// foreignSpace is a caught up Space with another writer device.
type foreignSpace struct {
	f   *adapterFixture
	eng world.Engine
	so  sobject.SharedObject
	dev *testDevice
	ops *heldOps
}

// newForeignSpace starts a Space whose operations resolve through heldOps,
// creates the object target, and adds another writer device.
func newForeignSpace(ctx context.Context, t *testing.T) *foreignSpace {
	t.Helper()
	ops := newHeldOps()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: ops.lookup}})
	eng, engRef := f.start(t)
	t.Cleanup(engRef.Release)
	s := &foreignSpace{f: f, eng: eng, so: f.space(t), ops: ops}
	if err := s.create(ctx, "target"); err != nil {
		t.Fatal(err)
	}
	s.dev = addDevice(t, s.so)
	return s
}

// create creates key through a local write.
func (s *foreignSpace) create(ctx context.Context, key string) error {
	return world.ExecTransaction(ctx, s.eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		return err
	})
}

// heldArrival has the other device write a mock operation on target, and
// returns once the driver's step is holding its replayed run, with the
// release that lets it go on.
func (s *foreignSpace) heldArrival(ctx context.Context, t *testing.T, msg string) func() {
	t.Helper()
	release := s.ops.arm()
	s.dev.write(t, s.so, mockOp(t, "target", msg, s.dev.id), false, false)
	select {
	case <-s.ops.entered:
	case <-ctx.Done():
		release()
		t.Fatal("the driver never stepped the other device's operation")
	}
	return release
}

// requireRefused attempts to create key and requires the typed replay wait,
// with nothing added to the order and no object written. It returns the wait.
func (s *foreignSpace) requireRefused(ctx context.Context, t *testing.T, key string) *sobject.BodyWait {
	t.Helper()
	before := orderLen(ctx, t, s.so)
	err := s.create(ctx, key)
	var wait *sobject.BodyWait
	if !errors.As(err, &wait) || !errors.Is(err, sobject.ErrReplayWaits) {
		t.Fatalf("write %s while another device's operation is unapplied = %v, want the typed replay wait", key, err)
	}
	if n := orderLen(ctx, t, s.so); n != before {
		t.Fatalf("order grew from %d to %d on a refused write", before, n)
	}
	if err := world.AccessEngine(ctx, s.eng, func(ctx context.Context, eng world.Engine) error {
		return lacksObject(ctx, eng, key)
	}); err != nil {
		t.Fatalf("refused write %s: %v", key, err)
	}
	return wait
}

// runForeignArrival has another device's operation arrive while this device
// is caught up, and holds the driver's step of it: the health shows catching
// up, and a local write is refused with the typed wait and writes nothing.
// Once the step applies the operation, the wait's Wake closes and the same
// write is admitted.
func runForeignArrival(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	s := newForeignSpace(ctx, t)
	health := spaceHealth(ctx, t, s.so)
	waitHealth(ctx, t, health, "caught up", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetCaughtUp() != nil
	})

	// The other device's operation arrives; the step holds it.
	release := s.heldArrival(ctx, t, "from the other device")
	waitHealth(ctx, t, health, "catching up", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetCatchingUp() != nil
	})
	wait := s.requireRefused(ctx, t, "during-step")

	// The step applies it: the wait ends and the write is admitted.
	release()
	select {
	case <-wait.Wake:
	case <-ctx.Done():
		t.Fatal("the wait's Wake never closed once the step applied the operation")
	}
	if err := s.create(ctx, "during-step"); err != nil {
		t.Fatalf("write after the step applied the operation: %v", err)
	}
	waitHealth(ctx, t, health, "caught up", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetCaughtUp() != nil
	})
}

// runRapidLocalWrites makes many local writes back to back with no other
// device's operation: every one is admitted.
func runRapidLocalWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	s := newForeignSpace(ctx, t)
	for i := range 64 {
		if err := s.create(ctx, "rapid-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("local write %d with no other device's operation: %v", i, err)
		}
	}
}

// runRapidWritesWithArrivals makes local writes back to back while the other
// device's operations arrive. While the step holds each arrival, every local
// write is refused with the typed wait and writes nothing; once the step
// applied it, every local write is admitted. Then local writes run beside
// unheld arrivals: each is admitted or meets the typed wait, whose Wake closes
// once the step applied the arrival, and the World ends with every admitted
// write and every arrival.
func runRapidWritesWithArrivals(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	s := newForeignSpace(ctx, t)
	for round := range 4 {
		release := s.heldArrival(ctx, t, "arrival "+strconv.Itoa(round))
		var wait *sobject.BodyWait
		for i := range 8 {
			wait = s.requireRefused(ctx, t, "held-"+strconv.Itoa(round)+"-"+strconv.Itoa(i))
		}
		release()
		select {
		case <-wait.Wake:
		case <-ctx.Done():
			t.Fatalf("round %d: the wait's Wake never closed once the step applied the operation", round)
		}
		for i := range 8 {
			if err := s.create(ctx, "applied-"+strconv.Itoa(round)+"-"+strconv.Itoa(i)); err != nil {
				t.Fatalf("round %d: local write %d after the step applied the operation: %v", round, i, err)
			}
		}
	}

	// Unheld arrivals beside local writes.
	ops := make([][]byte, 16)
	for i := range ops {
		ops[i] = createOp(t, "remote-"+strconv.Itoa(i))
	}
	host := s.so.(sobject.InviteHost).GetSOHost()
	arrived := make(chan error, 1)
	go func() {
		for _, op := range ops {
			err := host.UpdateSOState(ctx, func(state *sobject.SOState) error {
				return s.dev.add(t, s.so, state, op, false)
			})
			if err != nil {
				arrived <- err
				return
			}
		}
		arrived <- nil
	}()
	var admitted []string
	refused := 0
	for i := 0; ; i++ {
		key := "beside-" + strconv.Itoa(i)
		err := s.create(ctx, key)
		switch {
		case err == nil:
			admitted = append(admitted, key)
		case errors.Is(err, sobject.ErrReplayWaits):
			// Wait for the step to apply the arrival before the next write.
			refused++
			var wait *sobject.BodyWait
			if !errors.As(err, &wait) {
				t.Fatalf("local write %s beside arrivals: replay wait without its Wake: %v", key, err)
			}
			select {
			case <-wait.Wake:
			case <-ctx.Done():
				t.Fatalf("local write %s beside arrivals: the wait's Wake never closed", key)
			}
		default:
			t.Fatalf("local write %s beside arrivals: %v", key, err)
		}
		select {
		case err := <-arrived:
			if err != nil {
				t.Fatal(err)
			}
		default:
			continue
		}
		break
	}
	t.Logf("beside arrivals: %d writes admitted, %d met the replay wait", len(admitted), refused)
	keys := slices.Concat(admitted, []string{"target"})
	for i := range ops {
		keys = append(keys, "remote-"+strconv.Itoa(i))
	}
	waitObjects(ctx, t, s.eng, keys...)
}
