package world_block

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

type accessOnlyWorldOp struct {
	*world_mock.MockWorldOp
}

func (o *accessOnlyWorldOp) ApplyWorldOp(
	ctx context.Context,
	_ *logrus.Entry,
	state world.WorldState,
	_ peer.ID,
) (bool, error) {
	return false, state.AccessWorldState(ctx, nil, func(*world.WorldAccess) error {
		return nil
	})
}

func TestEngineAccessWorldStateSameBucketConcurrentClose(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("requires concurrent execution")
	}

	ctx := context.Background()
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

	const rounds = 32
	const workers = 16
	for range rounds {
		eng, err := NewEngine(ctx, le, base, nil, nil, false)
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		entered := make(chan struct{})
		var enteredOnce sync.Once
		var wg sync.WaitGroup
		var panicMu sync.Mutex
		var panics []any
		for range workers {
			wg.Go(func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						panicMu.Lock()
						panics = append(panics, recovered)
						panicMu.Unlock()
					}
				}()
				<-start
				for {
					err := eng.AccessWorldState(ctx, nil, func(*world.WorldAccess) error {
						enteredOnce.Do(func() { close(entered) })
						return nil
					})
					if err == ErrEngineClosed {
						return
					}
					if err != nil {
						t.Errorf("access failed: %v", err)
						return
					}
				}
			})
		}
		close(start)
		<-entered
		if err := eng.Close(); err != nil {
			t.Fatal(err)
		}
		wg.Wait()

		panicMu.Lock()
		panicsFound := append([]any(nil), panics...)
		panicMu.Unlock()
		if len(panicsFound) != 0 {
			t.Fatalf("concurrent close caused %d panic(s), first: %v", len(panicsFound), panicsFound[0])
		}
	}
}

func TestEngineAccessWorldStateReferencesAndCallbackBoundary(t *testing.T) {
	ctx := context.Background()
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

	eng, err := NewEngine(ctx, le, base, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	testAccess := func(ref *bucket.ObjectRef) {
		t.Helper()
		if err := eng.AccessWorldState(ctx, ref, func(cursor *world.WorldAccess) error {
			if !eng.rmtx.TryLock() {
				t.Fatal("callback ran while Engine.rmtx was held")
			}
			eng.rmtx.Unlock()
			if cursor == nil {
				t.Fatal("callback received nil cursor")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	testAccess(nil)
	testAccess(&bucket.ObjectRef{})
}
func TestTransactionWorldOperationDoesNotReenterEngineLock(t *testing.T) {
	ctx := context.Background()
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
	eng, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	eng.rmtx.Lock()
	done := make(chan error, 1)
	go func() {
		_, _, err := tx.ApplyWorldOp(ctx, &accessOnlyWorldOp{
			MockWorldOp: world_mock.NewMockWorldOp("lock-reentry", "access"),
		}, "")
		done <- err
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		eng.rmtx.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-timer.C:
		eng.rmtx.Unlock()
		<-done
		t.Fatal("transaction world operation reentered Engine.rmtx")
	}
}

func TestEngineTxAccessWorldStateNilPinsOpeningRoot(t *testing.T) {
	ctx := context.Background()
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

	eng, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	writeTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	objectBlockRef, _, err := base.PutBlock(ctx, []byte("object root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeTx.CreateObject(ctx, "pinned", &bucket.ObjectRef{RootRef: objectBlockRef}); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	openingRoot := eng.GetRootRef()

	readTx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	if !readTx.GetReadOnly() {
		t.Fatal("read transaction reported writable")
	}
	if _, err := readTx.CreateObject(ctx, "read-only-write", &bucket.ObjectRef{}); err != tx.ErrNotWrite {
		t.Fatalf("read-only write error = %v, want %v", err, tx.ErrNotWrite)
	}
	readObj, err := world.MustGetObject(ctx, readTx, "pinned")
	if err != nil {
		t.Fatal(err)
	}
	objectRoot, _, err := readObj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []*bucket.ObjectRef{nil, {}} {
		if err := readObj.AccessWorldState(ctx, ref, func(access *world.WorldAccess) error {
			if got := access.Cursor().GetRefWithOpArgs(); !got.GetRootRef().EqualsRef(objectRoot.GetRootRef()) {
				t.Fatalf("object access root = %v, want %v", got, objectRoot)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	ownedObjectRoot, err := readObj.BuildOwnedLookupCursor(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ownedObjectRoot.Cursor().GetRefWithOpArgs(); !got.GetRootRef().EqualsRef(objectRoot.GetRootRef()) {
		t.Fatalf("owned object root = %v, want %v", got, objectRoot)
	}
	ownedObjectRoot.Release()
	assertPinned := func(label string) {
		t.Helper()
		if err := readTx.AccessWorldState(ctx, nil, func(access *world.WorldAccess) error {
			if got := access.Cursor().GetRefWithOpArgs(); !got.EqualsRef(openingRoot) {
				t.Fatalf("%s root = %v, want opening root %v", label, got, openingRoot)
			}
			return nil
		}); err != nil {
			t.Fatalf("%s access: %v", label, err)
		}
	}
	assertPinned("before head movement")

	writeTx, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := world.MustGetObject(ctx, writeTx, "pinned")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj.IncrementRev(ctx); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	movingRoot := eng.GetRootRef()
	if movingRoot.EqualsRef(openingRoot) {
		t.Fatal("engine head did not move")
	}
	if err := eng.AccessWorldState(ctx, nil, func(access *world.WorldAccess) error {
		if got := access.Cursor().GetRefWithOpArgs(); !got.EqualsRef(movingRoot) {
			t.Fatalf("engine nil root = %v, want moving root %v", got, movingRoot)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.AccessWorldState(ctx, &bucket.ObjectRef{}, func(access *world.WorldAccess) error {
		if !access.Cursor().GetRef().GetEmpty() {
			t.Fatalf("allocated-empty engine ref selected moving root: %v", access.Cursor().GetRef())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertPinned("after head movement")

	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	assertPinned("after engine close")
}
