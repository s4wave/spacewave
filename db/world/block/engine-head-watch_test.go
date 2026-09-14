package world_block

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestEngineRevisionWaitersShareHeadWatch checks that external publication
// reaches every waiter through one engine-owned coordinator subscription.
func TestEngineRevisionWaitersShareHeadWatch(t *testing.T) {
	// Bound failures without using elapsed time to synchronize publication.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	// Independent engines share durable blocks and the published head.
	writer := newRetirementTestEngine(t, ctx)
	coordinator := &countingHeadCoordinator{
		Coordinator: coord_inmem.NewCoordinator(),
		started:     make(chan struct{}),
	}
	scope := coord.Scope{VolumeID: "head-watch-volume", ObjectStoreID: "head-watch-store"}
	root := writer.baseRoot.Clone()
	t.Cleanup(root.Release)
	constructorCtx, constructorCancel := context.WithCancel(ctx)
	t.Cleanup(constructorCancel)
	reader, err := NewEngine(constructorCtx, writer.le, root, world_mock.LookupMockOp, nil, false,
		WithWriteCoordinator(coordinator, scope, nil,
			func(context.Context) (*bucket.ObjectRef, error) { return writer.GetRootRef(), nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	constructorCancel()

	// Every waiter observes the same future revision.
	const waiterCount = 12
	results := make(chan error, waiterCount)
	for range waiterCount {
		go func() {
			seqno, err := reader.WaitSeqno(ctx, 1)
			if err == nil && seqno < 1 {
				t.Errorf("wait returned revision %d before publication", seqno)
			}
			results <- err
		}()
	}
	select {
	case <-coordinator.started:
	case <-ctx.Done():
		t.Fatal("coordinator subscription did not start")
	}

	// Publish through another engine and its normal coordinator event.
	ws := world.NewEngineWorldState(writer, true)
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, "head-watch/example", func(cursor *block.Cursor) error {
		cursor.SetBlock(block_mock.NewExample("before"), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.ApplyWorldOp(ctx, world_mock.NewMockWorldOp("head-watch/example", "after"), ""); err != nil {
		t.Fatal(err)
	}
	lease, err := coordinator.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Release(context.Background()) })
	if _, err := lease.Publish(ctx, coord.Event{RootChanged: writer.GetRootRef()}); err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatal(err)
	}
	for range waiterCount {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}

	// Subscription ownership ends with the engine, independently of waiter count.
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if got := coordinator.opened.Load(); got != 1 {
		t.Fatalf("coordinator subscriptions = %d, want one shared subscription", got)
	}
	if got := coordinator.closed.Load(); got != 1 {
		t.Fatalf("closed coordinator subscriptions = %d, want one", got)
	}
}

// TestEngineCloseJoinsHeadRefresh checks that a blocked coordinator refresh is
// canceled and its subscription released before Engine.Close returns.
func TestEngineCloseJoinsHeadRefresh(t *testing.T) {
	// Keep the real subscription blocked in its durable refresh callback.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	coordinator := &countingHeadCoordinator{Coordinator: coord_inmem.NewCoordinator()}
	entered := make(chan struct{})
	returned := make(chan struct{})
	engine := newRetirementTestEngine(t, ctx,
		WithWriteCoordinator(coordinator, coord.Scope{VolumeID: "close-watch", ObjectStoreID: "store"}, nil,
			func(ctx context.Context) (*bucket.ObjectRef, error) {
				close(entered)
				<-ctx.Done()
				close(returned)
				return nil, ctx.Err()
			}),
	)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("head watcher did not start")
	}

	// Close must cancel the callback, join it, and release its coordinator watch.
	closed := make(chan error, 1)
	go func() { closed <- engine.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Engine.Close did not cancel its head refresh")
	}
	select {
	case <-returned:
	default:
		t.Fatal("Engine.Close returned before its head refresh")
	}
	if got := coordinator.closed.Load(); got != 1 {
		t.Fatalf("closed coordinator subscriptions = %d, want one", got)
	}
	if _, err := engine.WaitSeqno(ctx, 1); !errors.Is(err, ErrEngineClosed) {
		t.Fatalf("revision wait after Close = %v, want ErrEngineClosed", err)
	}
}

// countingHeadCoordinator measures subscriptions around the real coordinator.
type countingHeadCoordinator struct {
	coord.Coordinator
	opened  atomic.Int32
	closed  atomic.Int32
	started chan struct{}
}

// Watch counts each successful subscription to the real coordinator.
func (c *countingHeadCoordinator) Watch(ctx context.Context, scope coord.Scope, after uint64) (coord.Watch, error) {
	watch, err := c.Coordinator.Watch(ctx, scope, after)
	if err != nil {
		return nil, err
	}
	if c.opened.Add(1) == 1 && c.started != nil {
		close(c.started)
	}
	return &countingHeadWatch{Watch: watch, closed: &c.closed}, nil
}

// countingHeadWatch observes completion of the underlying watch cleanup.
type countingHeadWatch struct {
	coord.Watch
	closed *atomic.Int32
}

// Close records the completed subscription release.
func (w *countingHeadWatch) Close() error {
	err := w.Watch.Close()
	w.closed.Add(1)
	return err
}
