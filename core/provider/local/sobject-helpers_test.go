package provider_local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/net/crypto"
)

// addsTestWorldOp reports whether next holds an operation other than an
// acknowledgment that prev lacks.
func addsTestWorldOp(prev, next *sobject.SOState) bool {
	had := make(map[string]bool, len(prev.GetOps()))
	for _, op := range prev.GetOps() {
		had[string(op.GetInner())] = true
	}
	for _, op := range next.GetOps() {
		if had[string(op.GetInner())] {
			continue
		}
		if inner, err := op.UnmarshalInner(); err == nil && !inner.IsAcknowledgment() {
			return true
		}
	}
	return false
}

// testAuthorNonce returns the nonce of author's newest operation in so's
// state.
func testAuthorNonce(ctx context.Context, t *testing.T, so *SharedObject, author string) uint64 {
	t.Helper()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nonce, _ := set.AuthorHead(author)
	return nonce
}

// mountCaptureKey marks the one caller whose promise selection the test gates.
type mountCaptureKey struct{}

// mountCapturePromise forces the completion arm of the resolved promise after
// retirement, an outcome allowed when both completion and replacement are ready.
type mountCapturePromise struct {
	// PromiseLike supplies the captured mount, already resolved.
	promise.PromiseLike[*SharedObject]
	// captured tells the test that the caller took the old promise.
	captured chan struct{}
	// resume lets the caller select the old completion before returning to
	// the tracker.
	resume chan struct{}
}

// AwaitWithCancelCh gates only the marked waiter and requires replacement
// before forcing its other ready arm. Background callers use the real select.
func (p *mountCapturePromise) AwaitWithCancelCh(ctx context.Context, replaced <-chan struct{}) (*SharedObject, error) {
	// Preserve normal behavior for every other waiter on the tracker.
	if ctx.Value(mountCaptureKey{}) == nil {
		return p.PromiseLike.AwaitWithCancelCh(ctx, replaced)
	}

	// Wait until the test retires the result and releases this waiter.
	close(p.captured)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.resume:
	}

	// Force the result arm only after both arms became ready.
	select {
	case <-replaced:
		return p.Await(ctx)
	default:
		return nil, errors.New("captured promise was not replaced")
	}
}

// mountOpenGate pauses the selected transaction after the mount's first
// check but before the underlying store opens it.
type mountOpenGate struct {
	object.ObjectStore
	write   bool
	entered chan struct{}
	resume  chan struct{}
}

func (g *mountOpenGate) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write == g.write {
		close(g.entered)
		select {
		case <-g.resume:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.ObjectStore.NewTransaction(ctx, write)
}

// mountWaitPromise signals that the caller is awaiting the pending result.
type mountWaitPromise struct {
	promise.PromiseLike[*SharedObject]
	waiting chan struct{}
}

// AwaitWithCancelCh observes the wait, then uses the real promise's select.
func (p *mountWaitPromise) AwaitWithCancelCh(ctx context.Context, replaced <-chan struct{}) (*SharedObject, error) {
	close(p.waiting)
	return p.PromiseLike.AwaitWithCancelCh(ctx, replaced)
}

// newTestRunSharedObject returns a SharedObject with only the host a run's
// result ends on retire.
func newTestRunSharedObject() *SharedObject {
	return &SharedObject{ctx: context.Background(), lsoHost: &LocalSOHost{
		stateSnapCtr: ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil),
	}}
}

// listTestSharedObjectWithoutState lists id with a block store and no stored
// state, as a create before creates stored a genesis did, without flipping
// DeleteStateEnv, which background mounts read.
func listTestSharedObjectWithoutState(ctx context.Context, t *testing.T, a *ProviderAccount, id string) *sobject.SharedObjectRef {
	relMtx, err := a.mtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relMtx()
	ref := sobject.NewSharedObjectRef(a.t.accountInfo.GetProviderId(), a.t.accountInfo.GetProviderAccountId(), id, SobjectBlockStoreID(id))
	if _, err := a.createBlockStoreLocked(ctx, SobjectBlockStoreID(id)); err != nil {
		t.Fatal(err)
	}
	list := a.soListCtr.GetValue().CloneVT()
	insertListEntry(list, newListEntry(ref, &sobject.SharedObjectMeta{BodyType: "test"}, "", nil))
	if err := a.writeSharedObjectList(ctx, list); err != nil {
		t.Fatal(err)
	}
	a.soListCtr.SetValue(list)
	return ref
}

// putTestStrayKey stores a local-state key under id with no state beside it.
func putTestStrayKey(ctx context.Context, t *testing.T, a *ProviderAccount, id string) {
	key := append(SobjectObjectStorePrefix(id), "ls/fake-store/fake-key"...)
	err := a.runSoObjectStoreTx(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, key, []byte("fake-value"))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// holdTestRunStart holds the next run of id before it takes its handle until
// the returned function is called.
func holdTestRunStart(t *testing.T, id string) func() {
	resume := make(chan struct{})
	var held, released sync.Once
	testHookRunStarting.Store(new(func(hid string) {
		if hid != id {
			return
		}
		hold := false
		held.Do(func() { hold = true })
		if hold {
			<-resume
		}
	}))
	release := func() { released.Do(func() { close(resume) }) }
	t.Cleanup(func() {
		release()
		testHookRunStarting.Store(nil)
	})
	return release
}

// expectTestResultCleared checks that the tracker holds no mount result: a
// set result returns at once, so a wait that reaches its deadline shows none.
// The await returns context.Canceled, not the context's own error, when its
// context ends.
func expectTestResultCleared(ctx context.Context, t *testing.T, tkr *sobjectTracker) {
	t.Helper()
	wctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	so, err := tkr.res.await(wctx)
	if !errors.Is(err, context.Canceled) || wctx.Err() == nil {
		t.Fatalf("the tracker holds a result: %v, %v", so != nil, err)
	}
}

// createTestSharedObject creates a space object with id.
func createTestSharedObject(ctx context.Context, t *testing.T, a *ProviderAccount, id string) *sobject.SharedObjectRef {
	t.Helper()
	ref, err := a.CreateSharedObject(ctx, id, &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// replaceTestState replaces the state of id with data over the state it
// stores now, as a transfer that read it would.
func replaceTestState(ctx context.Context, t *testing.T, a *ProviderAccount, id string, owner crypto.PrivKey, data []byte) {
	t.Helper()
	stored, err := a.ReadStoredSharedObject(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	origin := ReplaceOrigin{Generation: stored.Entry.GetGeneration(), State: stored.State, Source: []byte("source")}
	if err := a.ReplaceSharedObjectState(ctx, a.le, id, owner, data, origin); err != nil {
		t.Fatal(err)
	}
}

// expectTestMountEnded waits until the mount of so ends.
func expectTestMountEnded(ctx context.Context, t *testing.T, so *SharedObject) {
	t.Helper()
	select {
	case <-so.ctx.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the mount did not reload")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

// mountTestSharedObject mounts ref and returns the local handle.
func mountTestSharedObject(ctx context.Context, t *testing.T, a *ProviderAccount, ref *sobject.SharedObjectRef) (*SharedObject, func()) {
	t.Helper()
	so, rel, err := a.MountSharedObject(ctx, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	return so.(*SharedObject), rel
}

// expectTestMountNotFound checks that a mount of ref is refused promptly.
func expectTestMountNotFound(ctx context.Context, t *testing.T, a *ProviderAccount, ref *sobject.SharedObjectRef) {
	t.Helper()
	mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, rel, err := a.MountSharedObject(mctx, ref, nil)
	if err == nil {
		rel()
		t.Fatal("a mount of the deleted object succeeded")
	}
	if !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("a mount of the deleted object returned %v", err)
	}
}

// writeTestStateData adopts a checkpoint holding data and waits until the
// handle's snapshot holds it.
func writeTestStateData(ctx context.Context, t *testing.T, so *SharedObject, data []byte) {
	t.Helper()
	if err := tryWriteTestStateData(ctx, so, data); err != nil {
		t.Fatal(err)
	}
	waitTestStateData(ctx, t, so, data)
}

// waitTestStateData waits until the handle's snapshot holds data. The host
// publishes the snapshot from its watch of the committed state, after the
// commit, so a read right after a write can still see the state before it.
func waitTestStateData(ctx context.Context, t *testing.T, so *SharedObject, data []byte) {
	t.Helper()
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := so.lsoHost.stateSnapCtr.WaitValueWithValidator(wctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		checkpoint, err := snap.GetCheckpoint(wctx)
		if err != nil {
			return false, err
		}
		return bytes.Equal(checkpoint.GetStateData(), data), nil
	}, nil)
	if err != nil {
		t.Fatalf("the snapshot did not show the write: %v", err)
	}
}

// tryWriteTestStateData adopts a checkpoint holding data, encrypted with the
// current key, through the handle's SOHost.
func tryWriteTestStateData(ctx context.Context, so *SharedObject, data []byte) error {
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	snapshot, err := so.GetSharedObjectState(wctx)
	if err != nil {
		return err
	}
	xfrm, err := snapshot.GetTransformer(wctx)
	if err != nil {
		return err
	}
	enc, err := xfrm.EncodeBlock(data)
	if err != nil {
		return err
	}
	id := so.GetSharedObjectID()
	return so.GetSOHost().UpdateSOState(wctx, func(next *sobject.SOState) error {
		checkpoint, err := next.BuildNextCheckpoint(id, so.GetPrivKey(), enc)
		if err != nil {
			return err
		}
		return next.AdoptCheckpoint(id, checkpoint)
	})
}

// tryWriteTestLocalKeys writes n keys to a local state store of the handle in
// one transaction.
func tryWriteTestLocalKeys(ctx context.Context, so *SharedObject, n int) error {
	local, rel, err := so.AccessLocalStateStore(ctx, "fake-store", nil)
	if err != nil {
		return err
	}
	defer rel()
	return kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return local.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		for i := range n {
			if err := tx.Set(ctx, fmt.Appendf(nil, "fake-key-%04d", i), []byte("fake-value")); err != nil {
				return err
			}
		}
		return nil
	})
}

// setTestDeleteStateOff selects the old delete behavior, or the new one, for
// the rest of the test.
func setTestDeleteStateOff(t *testing.T, off bool) {
	prev := deleteStateOff
	deleteStateOff = off
	t.Cleanup(func() { deleteStateOff = prev })
}

// readTestStateData returns the state data of the handle's checkpoint.
func readTestStateData(ctx context.Context, t *testing.T, so *SharedObject) []byte {
	t.Helper()
	snapshot, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := snapshot.GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint.GetStateData()
}

// countTestKeys counts the account object store keys under prefix.
func countTestKeys(ctx context.Context, t *testing.T, a *ProviderAccount, prefix []byte) int {
	t.Helper()
	store, rel, err := a.buildSoObjectStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	var n int
	err = kvtx.RunTransaction(ctx, false, func(ctx context.Context) (kvtx.Tx, error) {
		return store.NewTransaction(ctx, false)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		n = 0
		return tx.ScanPrefixKeys(ctx, prefix, func([]byte) error {
			n++
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// testListed reports whether the account lists id.
func testListed(a *ProviderAccount, id string) bool {
	for _, entry := range a.soListCtr.GetValue().GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return true
		}
	}
	return false
}

// requireNoReleaseMark fails if id has a release mark.
func requireNoReleaseMark(ctx context.Context, t *testing.T, a *ProviderAccount, id string) {
	if n := countTestKeys(ctx, t, a, SobjectReleaseMarkKey(id)); n != 0 {
		t.Fatalf("%s has a release mark, want none", id)
	}
}

// rehearsalBucket returns the bucket of id's block store.
func rehearsalBucket(a *ProviderAccount, id string) string {
	return BlockStoreBucketID(a.GetProviderID(), a.GetAccountID(), SobjectBlockStoreID(id))
}

// requireRehearsalRoot fails unless id's "rehearsal" root is held as want.
func requireRehearsalRoot(ctx context.Context, t *testing.T, a *ProviderAccount, id string, want bool) {
	root, err := a.GetVolume().GetBucketRoot(ctx, rehearsalBucket(a, id), "rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	if (root != nil) != want {
		t.Fatalf("%s's rehearsal root held is %v, want %v", id, root != nil, want)
	}
}

// waitDeleteResume waits for the account's delete resume to exit. With
// notRunning set it returns at once when the resume never started.
func waitDeleteResume(ctx context.Context, t *testing.T, a *ProviderAccount, notRunning bool) {
	if err := a.deleteResume.WaitExited(ctx, notRunning, nil); err != nil {
		t.Fatal(err)
	}
}

// putRehearsalTree stores a two-block tree under the named root root of the
// object's block store: a parent recording id over a leaf recording id and a
// leaf every object shares. It returns the blocks only this object reaches,
// and the shared leaf.
func putRehearsalTree(ctx context.Context, t *testing.T, so *SharedObject, id, root string) ([]*block.BlockRef, *block.BlockRef) {
	t.Helper()
	leaf := []byte("fake-leaf-" + id)
	shared := []byte("fake-shared-leaf")
	parent := []byte("fake-parent-" + id)
	leafRef, err := block.BuildBlockRef(leaf, nil)
	if err != nil {
		t.Fatal(err)
	}
	sharedRef, err := block.BuildBlockRef(shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	parentRef, err := block.BuildBlockRef(parent, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = so.GetBlockStore().SetRoots(ctx, []*block.PutBatchEntry{
		{Ref: leafRef, Data: leaf, RefsKnown: true},
		{Ref: sharedRef, Data: shared, RefsKnown: true},
		{Ref: parentRef, Data: parent, Refs: []*block.BlockRef{leafRef, sharedRef}, RefsKnown: true},
	}, []block.NamedRoot{{Name: root, Ref: parentRef}})
	if err != nil {
		t.Fatal(err)
	}
	return []*block.BlockRef{parentRef, leafRef}, sharedRef
}
