package world_block

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

func TestEngineRootPublicationUnlocksBeforeReadTransactionDrain(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()

	engine, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	writer, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()
	if _, err := writer.CreateObject(ctx, "retirement/deadlock", nil); err != nil {
		t.Fatal(err)
	}

	locked := engine.bcast.Lock()
	oldRead := engine.head.readTx
	locked.Unlock()
	if oldRead == nil {
		t.Fatal("engine has no shared read transaction")
	}
	holdRead, err := oldRead.rmtx.Lock(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	readHeld := true
	defer func() {
		if readHeld {
			holdRead()
		}
	}()

	commitEntered := make(chan struct{})
	locked = engine.bcast.Lock()
	engine.commitFn = func(context.Context, *bucket.ObjectRef, *bucket.ObjectRef) error {
		close(commitEntered)
		return nil
	}
	locked.Unlock()

	commitDone := make(chan error, 1)
	go func() {
		_, err := writer.CommitBlockTransaction(ctx)
		commitDone <- err
	}()

	select {
	case <-commitEntered:
	case <-ctx.Done():
		t.Fatalf("commit did not reach root publication: %v", ctx.Err())
	}

	reentered := make(chan struct{})
	go func() {
		engine.GetRootRef()
		close(reentered)
	}()

	var reentryErr error
	select {
	case <-reentered:
	case <-time.After(500 * time.Millisecond):
		reentryErr = context.DeadlineExceeded
	}

	readHeld = false
	holdRead()
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	if reentryErr != nil {
		t.Fatal("root publication held Engine.bcast while draining the old read transaction")
	}
}

func TestEngineConcurrentCloseWaitsForFinalBroadcast(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	engine := newRetirementTestEngine(t, ctx)

	// Remove the head before Close so final baseRoot publication is the only
	// broadcast that can release concurrent Close waiters.
	locked := engine.bcast.Lock()
	retirement := engine.beginRetirementLocked(engineRetirement{head: engine.head})
	engine.head = nil
	locked.Unlock()
	engine.drainRetirement(ctx, retirement)

	// Queue every closer behind one held guard. The first closer releases the
	// guard before baseRoot release, allowing the rest to subscribe to completion.
	locked = engine.bcast.Lock()
	const closers = 16
	start := make(chan struct{})
	ready := make(chan struct{}, closers)
	done := make(chan error, closers)
	for range closers {
		go func() {
			ready <- struct{}{}
			<-start
			done <- engine.Close()
		}()
	}
	for range closers {
		<-ready
	}
	close(start)
	for range 100 {
		runtime.Gosched()
	}
	locked.Unlock()

	for i := range closers {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Close %d: %v", i+1, err)
			}
		case <-ctx.Done():
			t.Fatalf("Close %d missed final broadcast: %v", i+1, ctx.Err())
		}
	}
}

func TestEngineHeadPublishesRootAndReadTransactionTogether(t *testing.T) {
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)

	for i := range 8 {
		writer, err := engine.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.CreateObject(ctx, "retirement/head/"+strconv.Itoa(i), nil); err != nil {
			writer.Discard()
			t.Fatal(err)
		}
		if _, err := writer.CommitBlockTransaction(ctx); err != nil {
			writer.Discard()
			t.Fatal(err)
		}

		locked := engine.bcast.Lock()
		head := engine.head
		if head == nil || head.root == nil || head.readTx == nil {
			locked.Unlock()
			t.Fatal("published an incomplete engine head")
		}
		rootRef := head.root.GetRef().GetRootRef()
		readRef := head.readTx.state.GetRootRef()
		paired := readRef.EqualsRef(rootRef)
		locked.Unlock()
		if !paired {
			t.Fatalf("published mixed root and read transaction after commit %d", i)
		}
	}
}

func TestEngineCloseDrainsCoordinatorSnapshot(t *testing.T) {
	ctx := t.Context()
	coordinator := coord_inmem.NewCoordinator()
	engine := newRetirementTestEngine(
		t,
		ctx,
		WithWriteCoordinator(
			coordinator,
			coord.Scope{VolumeID: "retirement-volume", ObjectStoreID: "retirement-store"},
			nil,
			func(context.Context) (*bucket.ObjectRef, error) { return nil, nil },
		),
	)

	snapshot, err := engine.NewBlockEngineTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	locked := engine.bcast.Lock()
	registered := len(engine.coordinatorTxs)
	readTx := snapshot.readTx
	locked.Unlock()
	if registered != 1 {
		t.Fatalf("coordinator snapshot registrations = %d, want 1", registered)
	}

	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if !snapshot.rel.Load() {
		t.Fatal("coordinator snapshot remained active after Engine.Close")
	}
	if readTx == nil || !readTx.state.discarded.Load() {
		t.Fatal("coordinator read transaction was not drained by Engine.Close")
	}
	if _, _, err := snapshot.GetObject(ctx, "after-close"); !errors.Is(err, tx.ErrDiscarded) {
		t.Fatalf("coordinator snapshot operation after Close = %v, want %v", err, tx.ErrDiscarded)
	}
	locked = engine.bcast.Lock()
	registered = len(engine.coordinatorTxs)
	locked.Unlock()
	if registered != 0 {
		t.Fatalf("coordinator snapshot registrations after Close = %d, want 0", registered)
	}
}

func TestEngineWriterLockReleasesAfterTransactionDrain(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	engine := newRetirementTestEngine(t, ctx)

	first, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	holdWrite, err := first.writeTx.rmtx.Lock(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	writeHeld := true
	defer func() {
		if writeHeld {
			holdWrite()
		}
	}()

	discardDone := make(chan struct{})
	go func() {
		first.Discard()
		close(discardDone)
	}()
	for {
		locked := engine.bcast.Lock()
		detached := engine.writeTx == nil
		locked.Unlock()
		if detached {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("first writer did not detach: %v", ctx.Err())
		default:
			runtime.Gosched()
		}
	}

	type writerResult struct {
		tx  *EngineTx
		err error
	}
	secondDone := make(chan writerResult, 1)
	go func() {
		second, err := engine.NewBlockEngineTransaction(ctx, true)
		secondDone <- writerResult{tx: second, err: err}
	}()
	select {
	case result := <-secondDone:
		if result.tx != nil {
			result.tx.Discard()
		}
		t.Fatalf("successor writer acquired before old transaction drain: %v", result.err)
	case <-time.After(100 * time.Millisecond):
	}

	writeHeld = false
	holdWrite()
	select {
	case <-discardDone:
	case <-ctx.Done():
		t.Fatalf("first writer did not finish discard: %v", ctx.Err())
	}
	select {
	case result := <-secondDone:
		if result.err != nil {
			t.Fatal(result.err)
		}
		result.tx.Discard()
	case <-ctx.Done():
		t.Fatalf("successor writer did not acquire after old transaction drain: %v", ctx.Err())
	}
}

func TestCoordinatorSnapshotDeregistersOnDiscard(t *testing.T) {
	ctx := t.Context()
	engine := newRetirementTestEngine(
		t,
		ctx,
		WithWriteCoordinator(
			coord_inmem.NewCoordinator(),
			coord.Scope{VolumeID: "retirement-volume", ObjectStoreID: "retirement-store"},
			nil,
			func(context.Context) (*bucket.ObjectRef, error) { return nil, nil },
		),
	)

	for i := range 8 {
		snapshot, err := engine.NewBlockEngineTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.Discard()
		locked := engine.bcast.Lock()
		registered := len(engine.coordinatorTxs)
		locked.Unlock()
		if registered != 0 {
			t.Fatalf("coordinator snapshot registrations after discard %d = %d, want 0", i, registered)
		}
	}
}

func TestEngineReleasesCoordinatorLeaseAndWriterLockAfterUnlock(t *testing.T) {
	ctx := t.Context()
	var engine *Engine
	checking := &lockCheckingCoordinator{
		Coordinator: coord_inmem.NewCoordinator(),
		engine:      func() *Engine { return engine },
	}
	engine = newRetirementTestEngine(
		t,
		ctx,
		WithWriteCoordinator(
			checking,
			coord.Scope{VolumeID: "retirement-volume", ObjectStoreID: "retirement-store"},
			nil,
			nil,
		),
	)

	writer, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	locked := engine.bcast.Lock()
	releaseWriter := engine.writeTxRel
	engine.writeTxRel = func() {
		if held, ok := engine.bcast.TryLock(); !ok {
			checking.writerReleaseUnderEngine = true
		} else {
			held.Unlock()
		}
		releaseWriter()
	}
	locked.Unlock()

	writer.Discard()
	if !checking.leaseReleased {
		t.Fatal("coordinator lease was not released")
	}
	if checking.leaseReleaseUnderEngine {
		t.Fatal("coordinator lease was released while Engine.bcast was held")
	}
	if checking.writerReleaseUnderEngine {
		t.Fatal("writer lock was released while Engine.bcast was held")
	}
}

func TestEngineCloseWaitsForInFlightCommitLease(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	coordinator := &blockingRefreshCoordinator{
		Coordinator: coord_inmem.NewCoordinator(),
		entered:     make(chan struct{}),
		proceed:     make(chan struct{}),
	}
	engine := newRetirementTestEngine(
		t,
		ctx,
		WithWriteCoordinator(
			coordinator,
			coord.Scope{VolumeID: "retirement-volume", ObjectStoreID: "retirement-store"},
			nil,
			nil,
		),
	)
	writer, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.CreateObject(ctx, "retirement/close-commit", nil); err != nil {
		writer.Discard()
		t.Fatal(err)
	}

	commitDone := make(chan error, 1)
	go func() {
		_, err := writer.CommitBlockTransaction(ctx)
		commitDone <- err
	}()
	select {
	case <-coordinator.entered:
	case <-ctx.Done():
		t.Fatalf("commit did not reach lease refresh: %v", ctx.Err())
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- engine.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Engine.Close returned before in-flight commit released its lease: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(coordinator.proceed)
	if err := <-commitDone; !errors.Is(err, tx.ErrDiscarded) {
		t.Fatalf("commit during Close = %v, want %v", err, tx.ErrDiscarded)
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatalf("Engine.Close did not join in-flight commit: %v", ctx.Err())
	}
}

type blockingRefreshCoordinator struct {
	coord.Coordinator
	entered chan struct{}
	proceed chan struct{}
}

func (c *blockingRefreshCoordinator) WaitAcquireWriteLease(
	ctx context.Context,
	scope coord.Scope,
) (coord.WriteLease, error) {
	lease, err := c.Coordinator.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, err
	}
	return &blockingRefreshLease{
		WriteLease: lease,
		entered:    c.entered,
		proceed:    c.proceed,
	}, nil
}

type blockingRefreshLease struct {
	coord.WriteLease
	refreshes int
	entered   chan struct{}
	proceed   chan struct{}
}

func (l *blockingRefreshLease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	l.refreshes++
	if l.refreshes == 2 {
		close(l.entered)
		select {
		case <-l.proceed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return l.WriteLease.Refresh(ctx)
}

func TestEngineCloseWaitsForPublishedCommitCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	coordinator := &blockingPublishCoordinator{
		Coordinator: coord_inmem.NewCoordinator(),
		entered:     make(chan struct{}),
		proceed:     make(chan struct{}),
	}
	engine := newRetirementTestEngine(
		t,
		ctx,
		WithWriteCoordinator(
			coordinator,
			coord.Scope{VolumeID: "retirement-volume", ObjectStoreID: "retirement-store"},
			nil,
			nil,
		),
	)
	writer, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.CreateObject(ctx, "retirement/close-publish", nil); err != nil {
		writer.Discard()
		t.Fatal(err)
	}

	commitDone := make(chan error, 1)
	go func() {
		_, err := writer.CommitBlockTransaction(ctx)
		commitDone <- err
	}()
	select {
	case <-coordinator.entered:
	case <-ctx.Done():
		t.Fatalf("commit did not reach coordinator publication: %v", ctx.Err())
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- engine.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Engine.Close returned before published commit cleanup: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(coordinator.proceed)
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatalf("Engine.Close did not join published commit cleanup: %v", ctx.Err())
	}
}

type blockingPublishCoordinator struct {
	coord.Coordinator
	entered chan struct{}
	proceed chan struct{}
}

func (c *blockingPublishCoordinator) WaitAcquireWriteLease(
	ctx context.Context,
	scope coord.Scope,
) (coord.WriteLease, error) {
	lease, err := c.Coordinator.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, err
	}
	return &blockingPublishLease{
		WriteLease: lease,
		entered:    c.entered,
		proceed:    c.proceed,
	}, nil
}

type blockingPublishLease struct {
	coord.WriteLease
	entered chan struct{}
	proceed chan struct{}
}

func (l *blockingPublishLease) Publish(
	ctx context.Context,
	event coord.Event,
) (*coord.Snapshot, error) {
	close(l.entered)
	select {
	case <-l.proceed:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return l.WriteLease.Publish(ctx, event)
}

type lockCheckingCoordinator struct {
	coord.Coordinator
	engine                   func() *Engine
	leaseReleased            bool
	leaseReleaseUnderEngine  bool
	writerReleaseUnderEngine bool
}

func (c *lockCheckingCoordinator) WaitAcquireWriteLease(
	ctx context.Context,
	scope coord.Scope,
) (coord.WriteLease, error) {
	lease, err := c.Coordinator.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return nil, err
	}
	return &lockCheckingLease{
		WriteLease: lease,
		release: func() {
			c.leaseReleased = true
			engine := c.engine()
			locked, ok := engine.bcast.TryLock()
			if !ok {
				c.leaseReleaseUnderEngine = true
				return
			}
			locked.Unlock()
		},
	}, nil
}

type lockCheckingLease struct {
	coord.WriteLease
	release func()
}

func (l *lockCheckingLease) Release(ctx context.Context) error {
	l.release()
	return l.WriteLease.Release(ctx)
}

func newRetirementTestEngine(
	t *testing.T,
	ctx context.Context,
	opts ...EngineOption,
) *Engine {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(base.Release)
	engine, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false, opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Error(err)
		}
	})
	return engine
}
