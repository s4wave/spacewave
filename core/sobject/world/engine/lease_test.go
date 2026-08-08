package sobject_world_engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/volume"
	alpha_testbed "github.com/s4wave/spacewave/testbed"
)

func TestWorldEngineLeaseLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	soA := &leaseTestSharedObject{
		testSharedObject: testSharedObject{
			blockStore: newTestBlockStore("provider-block-store-a", tb.Volume),
		},
		id:  "object-a",
		vol: tb.Volume,
	}
	engineA := &Controller{bus: tb.Bus, engineID: "world-x"}
	leaseA, _, err := engineA.acquireWorldEngineLease(ctx, soA)
	if err != nil {
		t.Fatal(err.Error())
	}

	engineB := &Controller{bus: tb.Bus, engineID: "world-y"}
	_, _, err = engineB.acquireWorldEngineLease(ctx, soA)
	if err == nil {
		t.Fatal("second acquisition for object-a succeeded")
	}

	soB := &leaseTestSharedObject{
		testSharedObject: testSharedObject{
			blockStore: newTestBlockStore("provider-block-store-b", tb.Volume),
		},
		id:  "object-b",
		vol: tb.Volume,
	}
	leaseB, _, err := engineB.acquireWorldEngineLease(ctx, soB)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := leaseB.Release(ctx); err != nil {
		t.Fatal(err.Error())
	}

	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err.Error())
	}
	leaseA, _, err = engineB.acquireWorldEngineLease(ctx, soA)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := leaseA.Release(ctx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestWorldEngineLeaseLossStopsEngineContext(t *testing.T) {
	ctx := context.Background()
	tb, err := alpha_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	lease := newTestWorldEngineLease()
	vol := &leaseTestVolume{Volume: tb.Volume, lease: lease, detectsLoss: true}
	so := &leaseTestSharedObject{
		testSharedObject: testSharedObject{
			blockStore: newTestBlockStore("provider-block-store-loss", tb.Volume),
		},
		id:  "object-loss",
		vol: vol,
	}
	c := &Controller{bus: tb.Bus, engineID: "world-loss"}
	acquired, detectsLoss, err := c.acquireWorldEngineLease(ctx, so)
	if err != nil {
		t.Fatal(err.Error())
	}

	engineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if !detectsLoss {
		t.Fatal("loss-reporting coordinator did not declare DetectsLoss")
	}
	watchWorldEngineLease(engineCtx, acquired, detectsLoss, cancel)

	lossErr := errors.New("lease renewal failed")
	lease.lose(lossErr)

	select {
	case <-engineCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("engine context was not canceled after lease loss")
	}
	if !errors.Is(acquired.Err(), lossErr) {
		t.Fatalf("lease error = %v, want %v", acquired.Err(), lossErr)
	}
}

func TestWorldEngineLeaseWithoutLossDetectionKeepsEngineContext(t *testing.T) {
	engineCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lease := newTestWorldEngineLease()
	watchWorldEngineLease(engineCtx, lease, false, cancel)
	lease.lose(errors.New("unreported lease loss"))

	select {
	case <-engineCtx.Done():
		t.Fatal("engine context canceled for a coordinator without loss detection")
	case <-time.After(20 * time.Millisecond):
	}
}

type leaseTestVolume struct {
	volume.Volume
	lease       coord.WriteLease
	detectsLoss bool
}

func (v *leaseTestVolume) Capability(context.Context, coord.Scope) (*coord.Capability, error) {
	return &coord.Capability{Supported: true, DetectsLoss: v.detectsLoss}, nil
}

func (v *leaseTestVolume) TryAcquireWriteLease(
	context.Context,
	coord.Scope,
) (coord.WriteLease, bool, error) {
	return v.lease, true, nil
}

type testWorldEngineLease struct {
	done chan struct{}
	err  error
}

func newTestWorldEngineLease() *testWorldEngineLease {
	return &testWorldEngineLease{done: make(chan struct{})}
}

func (l *testWorldEngineLease) Done() <-chan struct{} {
	return l.done
}

func (l *testWorldEngineLease) Err() error {
	return l.err
}

func (*testWorldEngineLease) Refresh(context.Context) (*coord.Snapshot, error) {
	return nil, coord.ErrUnsupported
}

func (*testWorldEngineLease) Publish(context.Context, coord.Event) (*coord.Snapshot, error) {
	return nil, coord.ErrUnsupported
}

func (l *testWorldEngineLease) Release(context.Context) error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}

func (l *testWorldEngineLease) lose(err error) {
	l.err = err
	close(l.done)
}

type leaseTestSharedObject struct {
	testSharedObject
	id  string
	vol volume.Volume
}

func (s *leaseTestSharedObject) GetSharedObjectID() string {
	return s.id
}

func (s *leaseTestSharedObject) GetBackingVolume() volume.Volume {
	return s.vol
}
