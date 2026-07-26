package sobject_world_engine

import (
	"context"
	"errors"
	"testing"
	"time"

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
	leaseA, err := engineA.acquireWorldEngineLease(ctx, soA)
	if err != nil {
		t.Fatal(err.Error())
	}

	engineB := &Controller{bus: tb.Bus, engineID: "world-y"}
	_, err = engineB.acquireWorldEngineLease(ctx, soA)
	var heldErr *volume.WorldEngineLeaseHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("second acquisition for object-a = %v, want WorldEngineLeaseHeldError", err)
	}
	if heldErr.Key != "object-a" {
		t.Fatalf("held lease key = %q, want object-a", heldErr.Key)
	}

	soB := &leaseTestSharedObject{
		testSharedObject: testSharedObject{
			blockStore: newTestBlockStore("provider-block-store-b", tb.Volume),
		},
		id:  "object-b",
		vol: tb.Volume,
	}
	leaseB, err := engineB.acquireWorldEngineLease(ctx, soB)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := leaseB.Release(); err != nil {
		t.Fatal(err.Error())
	}

	if err := leaseA.Release(); err != nil {
		t.Fatal(err.Error())
	}
	leaseA, err = engineB.acquireWorldEngineLease(ctx, soA)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := leaseA.Release(); err != nil {
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
	vol := &leaseTestVolume{Volume: tb.Volume, lease: lease}
	so := &leaseTestSharedObject{
		testSharedObject: testSharedObject{
			blockStore: newTestBlockStore("provider-block-store-loss", tb.Volume),
		},
		id:  "object-loss",
		vol: vol,
	}
	c := &Controller{bus: tb.Bus, engineID: "world-loss"}
	acquired, err := c.acquireWorldEngineLease(ctx, so)
	if err != nil {
		t.Fatal(err.Error())
	}

	engineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	watchWorldEngineLease(engineCtx, acquired, cancel)

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

type leaseTestVolume struct {
	volume.Volume
	lease volume.WorldEngineLease
}

func (v *leaseTestVolume) AcquireWorldEngineLease(
	context.Context,
	string,
) (volume.WorldEngineLease, error) {
	return v.lease, nil
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

func (l *testWorldEngineLease) Release() error {
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
