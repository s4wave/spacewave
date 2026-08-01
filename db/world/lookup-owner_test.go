package world

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	bucket_mock "github.com/s4wave/spacewave/db/bucket/mock"
)

func newLookupOwnerTestCursor(bkt bucket.BucketOps, bucketID string, released func()) *bucket_lookup.Cursor {
	return bucket_lookup.NewCursorWithRelease(
		context.Background(),
		nil,
		nil,
		nil,
		bkt,
		nil,
		&bucket.ObjectRef{BucketId: bucketID},
		&bucket.BucketOpArgs{BucketId: bucketID},
		nil,
		released,
	)
}

func writeLookupOwnerTestBlock(t *testing.T, ctx context.Context, access *WorldAccess, msg string) *block.BlockRef {
	t.Helper()
	btx, bcs := access.BuildTransaction(nil)
	bcs.SetBlock(&block_mock.Example{Msg: msg}, true)
	ref, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatalf("write block: %v", err)
	}
	return ref
}

func TestWorldStorageOwnerRoutesSelectedStores(t *testing.T) {
	ctx := context.Background()
	source := bucket_mock.NewMockBucket("source", nil)
	destination := bucket_mock.NewMockBucket("destination", nil)
	sourceOverlay := block.NewBufferedStore(ctx, source)
	base := newLookupOwnerTestCursor(source, "source", nil)
	storage := NewWorldStorageFromCursorWithStore(base, sourceOverlay).(*cursorWorldStorage)
	defer storage.retire()

	if err := storage.AccessWorldState(ctx, nil, func(access *WorldAccess) error {
		pendingRef := writeLookupOwnerTestBlock(t, ctx, access, "pending")
		if _, found, err := source.GetBlock(ctx, pendingRef); err != nil || found {
			t.Fatalf("raw source lookup before Sync = (%v, %v), want not found", found, err)
		}
		if _, found, err := sourceOverlay.GetBlock(ctx, pendingRef); err != nil || !found {
			t.Fatalf("source overlay lookup = (%v, %v), want found", found, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("same-bucket access: %v", err)
	}

	rawRef, _, err := source.PutBlock(ctx, []byte("raw fallthrough"), nil)
	if err != nil {
		t.Fatalf("raw put: %v", err)
	}
	if err := storage.AccessWorldState(ctx, nil, func(access *WorldAccess) error {
		_, bcs := access.BuildTransactionAtRef(nil, rawRef)
		data, found, err := bcs.Fetch(ctx)
		if err != nil || !found || string(data) != "raw fallthrough" {
			t.Fatalf("overlay fallthrough = (%q, %v, %v)", data, found, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("raw fallthrough access: %v", err)
	}

	destinationCursor := newLookupOwnerTestCursor(destination, "destination", nil)
	crossAccess := &WorldAccess{cursor: destinationCursor, storage: storage}
	crossRef := writeLookupOwnerTestBlock(t, ctx, crossAccess, "destination")
	if _, found, err := destination.GetBlock(ctx, crossRef); err != nil || !found {
		t.Fatalf("destination lookup = (%v, %v), want found", found, err)
	}
	if _, found, err := sourceOverlay.GetBlock(ctx, crossRef); err != nil || found {
		t.Fatalf("source overlay cross-bucket lookup = (%v, %v), want not found", found, err)
	}

	explicitSameCursor := newLookupOwnerTestCursor(destination, "source", nil)
	explicitSameAccess := &WorldAccess{cursor: explicitSameCursor, storage: storage}
	explicitSameRef := writeLookupOwnerTestBlock(t, ctx, explicitSameAccess, "explicit same bucket")
	if _, found, err := sourceOverlay.GetBlock(ctx, explicitSameRef); err != nil || !found {
		t.Fatalf("explicit same-bucket overlay lookup = (%v, %v), want found", found, err)
	}
	if _, found, err := destination.GetBlock(ctx, explicitSameRef); err != nil || found {
		t.Fatalf("explicit same-bucket destination lookup = (%v, %v), want not found", found, err)
	}
}

func TestWorldStorageBuilderOwnedCursorFollowsRequestedRef(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("source", nil)
	rootRef, _, err := bkt.PutBlock(ctx, []byte("requested root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	base := newLookupOwnerTestCursor(bkt, "source", nil)
	storage := NewCursorWorldStorage(func(context.Context) (*bucket_lookup.Cursor, error) {
		return base.Clone(), nil
	})
	defer RetireWorldStorage(storage)

	requested := &bucket.ObjectRef{BucketId: "source", RootRef: rootRef}
	owned, err := storage.BuildOwnedLookupCursor(ctx, requested)
	if err != nil {
		t.Fatalf("build requested cursor: %v", err)
	}
	defer owned.Release()
	if !owned.Cursor().GetRef().EqualsRef(requested) {
		t.Fatalf("owned cursor ref = %v, want %v", owned.Cursor().GetRef(), requested)
	}
}

func TestOwnedLookupCursorChildOutlivesParentAndStorage(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("source", nil)
	var releases atomic.Int32
	base := newLookupOwnerTestCursor(bkt, "source", func() { releases.Add(1) })
	storage := NewWorldStorageFromCursorWithStore(base, bkt).(*cursorWorldStorage)

	parent, err := storage.BuildOwnedLookupCursor(ctx, nil)
	if err != nil {
		t.Fatalf("build parent: %v", err)
	}
	child, err := parent.FollowRef(ctx, &bucket.ObjectRef{BucketId: "source"})
	if err != nil {
		t.Fatalf("follow child: %v", err)
	}
	storage.retire()
	parent.Release()
	parent.Release()
	if releases.Load() != 0 {
		t.Fatalf("release count with child alive = %d, want 0", releases.Load())
	}
	if child.Cursor() == nil || child.Cursor().GetOpArgs().GetBucketId() != "source" {
		t.Fatal("child cursor did not remain usable")
	}
	child.Release()
	child.Release()
	if releases.Load() != 1 {
		t.Fatalf("final release count = %d, want 1", releases.Load())
	}
	if _, err := storage.BuildOwnedLookupCursor(ctx, nil); !errors.Is(err, ErrWorldStorageUnavailable) {
		t.Fatalf("build after retirement = %v, want %v", err, ErrWorldStorageUnavailable)
	}
}

func TestOwnedLookupCursorCloneRetainsAuthorityChain(t *testing.T) {
	bkt := bucket_mock.NewMockBucket("source", nil)
	var sourceReleases atomic.Int32
	source := newLookupOwnerTestCursor(bkt, "source", func() { sourceReleases.Add(1) })
	_, sourceLease, err := newLookupAuthority(source)
	if err != nil {
		t.Fatal(err)
	}

	var destinationReleases atomic.Int32
	destination := newLookupOwnerTestCursor(bkt, "destination", func() { destinationReleases.Add(1) })
	destinationAuth, destinationLease, err := newLookupAuthorityWithRelease(destination, func() {
		destination.Release()
		sourceLease.Release()
	})
	if err != nil {
		sourceLease.Release()
		t.Fatal(err)
	}
	base := newOwnedLookupCursor(destination.Clone(), destinationAuth, destinationLease)
	storage := NewWorldStorageFromOwnedLookupCursor(base, bkt).(*cursorWorldStorage)
	child, err := storage.BuildOwnedLookupCursor(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	storage.retire()
	if got := destinationReleases.Load(); got != 0 {
		t.Fatalf("destination release count with child alive = %d, want 0", got)
	}
	if got := sourceReleases.Load(); got != 0 {
		t.Fatalf("source release count with child alive = %d, want 0", got)
	}
	child.Release()
	if got := destinationReleases.Load(); got != 1 {
		t.Fatalf("destination release count = %d, want 1", got)
	}
	if got := sourceReleases.Load(); got != 1 {
		t.Fatalf("source release count = %d, want 1", got)
	}
}

func TestAccessObjectNilStartsFromEmptyRoot(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("source", nil)
	existingRef, _, err := bkt.PutBlock(ctx, []byte("existing root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	base := newLookupOwnerTestCursor(bkt, "source", nil)
	base.SetRootRef(existingRef)
	storage := NewWorldStorageFromCursorWithStore(base, bkt)
	defer RetireWorldStorage(storage)

	var callbackStartedEmpty bool
	out, err := AccessObject(ctx, storage.AccessWorldState, nil, func(bcs *block.Cursor) error {
		callbackStartedEmpty = bcs.GetRef().GetEmpty()
		bcs.SetBlock(&block_mock.Example{Msg: "created"}, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !callbackStartedEmpty {
		t.Fatal("AccessObject(nil) callback did not start from an empty root")
	}
	if out.GetRootRef().GetEmpty() || out.GetRootRef().EqualsRef(existingRef) {
		t.Fatalf("created root = %v, want a new non-empty root", out)
	}
}

func TestWorldStorageBorrowedAccessFinalizesEveryExit(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("source", nil)
	var releases atomic.Int32
	storage := NewWorldStorageFromCursorWithStore(
		newLookupOwnerTestCursor(bkt, "source", func() { releases.Add(1) }),
		bkt,
	).(*cursorWorldStorage)

	assertActive := func(label string) {
		t.Helper()
		if releases.Load() != 0 {
			t.Fatalf("%s released authority while owner remains active", label)
		}
	}
	if err := storage.AccessWorldState(ctx, nil, func(*WorldAccess) error { return nil }); err != nil {
		t.Fatalf("successful access: %v", err)
	}
	assertActive("success")
	wantErr := errors.New("callback failure")
	if err := storage.AccessWorldState(ctx, nil, func(*WorldAccess) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("callback error = %v, want %v", err, wantErr)
	}
	assertActive("error")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected callback panic")
			}
		}()
		_ = storage.AccessWorldState(ctx, nil, func(*WorldAccess) error { panic("callback panic") })
	}()
	assertActive("panic")
	storage.retire()
	if releases.Load() != 1 {
		t.Fatalf("final release count = %d, want 1", releases.Load())
	}
}

func TestWorldStorageOwnedBuilderRetireRejectsInFlightCursor(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("source", nil)
	started := make(chan struct{})
	unblock := make(chan struct{})
	var releases atomic.Int32
	storage := NewCursorWorldStorage(func(context.Context) (*bucket_lookup.Cursor, error) {
		close(started)
		<-unblock
		return newLookupOwnerTestCursor(bkt, "source", func() { releases.Add(1) }), nil
	}).(*cursorWorldStorage)

	result := make(chan error, 1)
	go func() {
		owned, err := storage.BuildOwnedLookupCursor(ctx, nil)
		if owned != nil {
			owned.Release()
		}
		result <- err
	}()
	<-started
	storage.retire()
	close(unblock)
	if err := <-result; !errors.Is(err, ErrWorldStorageUnavailable) {
		t.Fatalf("build completing after retire = %v, want %v", err, ErrWorldStorageUnavailable)
	}
	if releases.Load() != 1 {
		t.Fatalf("release count = %d, want 1", releases.Load())
	}
}

func TestWorldStorageRawBuilderDoesNotBlockRetire(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("source", nil)
	started := make(chan struct{})
	unblock := make(chan struct{})
	var releases atomic.Int32
	storage := NewCursorWorldStorage(func(context.Context) (*bucket_lookup.Cursor, error) {
		close(started)
		<-unblock
		return newLookupOwnerTestCursor(bkt, "source", func() { releases.Add(1) }), nil
	}).(*cursorWorldStorage)

	result := make(chan error, 1)
	go func() {
		raw, err := storage.BuildStorageCursor(ctx)
		if raw != nil {
			raw.Release()
		}
		result <- err
	}()
	<-started
	retired := make(chan struct{})
	go func() {
		storage.retire()
		close(retired)
	}()
	select {
	case <-retired:
	case <-time.After(time.Second):
		close(unblock)
		<-retired
		t.Fatal("raw cursor builder blocked storage retirement")
	}
	close(unblock)
	if err := <-result; !errors.Is(err, ErrWorldStorageUnavailable) {
		t.Fatalf("raw build completing after retire = %v, want %v", err, ErrWorldStorageUnavailable)
	}
	if releases.Load() != 1 {
		t.Fatalf("release count = %d, want 1", releases.Load())
	}
}

func TestWorldStorageConcurrentAccessAndRetire(t *testing.T) {
	ctx := context.Background()
	bkt := bucket_mock.NewMockBucket("source", nil)
	var releases atomic.Int32
	storage := NewWorldStorageFromCursorWithStore(
		newLookupOwnerTestCursor(bkt, "source", func() { releases.Add(1) }),
		bkt,
	).(*cursorWorldStorage)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			<-start
			owned, err := storage.BuildOwnedLookupCursor(ctx, nil)
			if err != nil {
				if !errors.Is(err, ErrWorldStorageUnavailable) {
					t.Errorf("build during retire: %v", err)
				}
				return
			}
			if owned.Cursor() == nil {
				t.Error("successful build returned nil cursor")
			}
			owned.Release()
		})
	}
	close(start)
	storage.retire()
	wg.Wait()
	if releases.Load() != 1 {
		t.Fatalf("release count = %d, want 1", releases.Load())
	}
	if _, err := storage.BuildOwnedLookupCursor(ctx, nil); !errors.Is(err, ErrWorldStorageUnavailable) {
		t.Fatalf("build after retire = %v, want %v", err, ErrWorldStorageUnavailable)
	}
}
