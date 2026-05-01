package cdn_sharedobject

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/db/bucket"

	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
)

const testSpaceID = "01kpftest0000000000000001"

// newTestSharedObject builds a CdnSharedObject wrapped around a CdnBlockStore
// whose pointer has been pre-populated with a synthetic SORoot carrying a
// single head ref. The block store's network side is unused because the test
// touches only the metadata / state-snapshot surface.
func newTestSharedObject(t *testing.T, seed *sobject.SORoot) *CdnSharedObject {
	t.Helper()
	bs, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: "https://example.invalid",
		SpaceID:    testSpaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if seed != nil {
		bs.SetPointer(&alpha_cdn.CdnRootPointer{
			SpaceId: testSpaceID,
			Root:    seed,
		})
	}
	so, err := NewCdnSharedObject(CdnSharedObjectOptions{
		SpaceID:    testSpaceID,
		BlockStore: bs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return so
}

func TestMetadataSurface(t *testing.T) {
	so := newTestSharedObject(t, nil)
	if got := so.GetSharedObjectID(); got != testSpaceID {
		t.Fatalf("unexpected shared object id: %q", got)
	}
	if got := so.GetDisplayName(); got != CdnDisplayName {
		t.Fatalf("unexpected display name: %q", got)
	}
	if !so.IsPublicRead() {
		t.Fatal("CDN mount must report public_read=true")
	}
	if meta := so.GetMeta(); meta.GetBodyType() != CdnBodyType {
		t.Fatalf("unexpected body type: %q", meta.GetBodyType())
	}
	if so.GetBlockStore() == nil {
		t.Fatal("block store must not be nil")
	}
	if so.GetBlockStore().GetID() != testSpaceID {
		t.Fatalf("block store id should be space id, got %q", so.GetBlockStore().GetID())
	}
}

func TestWritePathsRejected(t *testing.T) {
	ctx := context.Background()
	so := newTestSharedObject(t, nil)
	if _, err := so.QueueOperation(ctx, []byte("x")); err == nil {
		t.Fatal("expected QueueOperation to error")
	}
	if _, _, err := so.WaitOperation(ctx, "local"); err == nil {
		t.Fatal("expected WaitOperation to error")
	}
	if err := so.ClearOperationResult(ctx, "local"); err == nil {
		t.Fatal("expected ClearOperationResult to error")
	}
	if err := so.ProcessOperations(ctx, false, nil); err == nil {
		t.Fatal("expected ProcessOperations to error")
	}
	if _, _, err := so.AccessLocalStateStore(ctx, "state", nil); err == nil {
		t.Fatal("expected AccessLocalStateStore to error")
	}
}

func TestSnapshotBeforeAndAfterPointer(t *testing.T) {
	ctx := context.Background()
	so := newTestSharedObject(t, nil)

	// Before any pointer is cached, the snapshot surfaces an empty queue
	// and a nil root inner so callers can distinguish "fresh Space" from
	// "decode error".
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ops, local, err := snap.GetOpQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 || len(local) != 0 {
		t.Fatalf("expected empty queues, got ops=%d local=%d", len(ops), len(local))
	}
	inner, err := snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if inner != nil {
		t.Fatalf("expected nil root inner before pointer is cached, got %+v", inner)
	}

	// After a pointer is cached, GetRootInner should decode the plain
	// SORootInner and expose the InnerState HeadRef.
	head := &bucket.ObjectRef{}
	innerState := &sobject_world_engine.InnerState{HeadRef: head}
	innerStateBytes, err := innerState.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	sori := &sobject.SORootInner{Seqno: 7, StateData: innerStateBytes}
	soriBytes, err := sori.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	so.bs.SetPointer(&alpha_cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Root:    &sobject.SORoot{Inner: soriBytes, InnerSeqno: 7},
	})

	decoded, err := snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if decoded == nil || decoded.GetSeqno() != 7 {
		t.Fatalf("decoded SORootInner mismatch: %+v", decoded)
	}

	decodedInner, err := so.GetHeadInnerState()
	if err != nil {
		t.Fatal(err)
	}
	if decodedInner == nil {
		t.Fatal("expected non-nil InnerState")
	}
}

func TestEmptyInitializedPointerHasNoHead(t *testing.T) {
	ctx := context.Background()
	so := newTestSharedObject(t, &sobject.SORoot{
		Inner:      []byte("not a plaintext SORootInner"),
		InnerSeqno: 1,
	})

	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := snap.GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if inner != nil {
		t.Fatalf("root inner = %+v, want nil for empty initialized CDN root", inner)
	}
	head, err := so.GetHeadInnerState()
	if err != nil {
		t.Fatal(err)
	}
	if head != nil {
		t.Fatalf("head inner state = %+v, want nil for empty initialized CDN root", head)
	}
}

func TestPackedPointerRejectsUndecodableRoot(t *testing.T) {
	so := newTestSharedObject(t, nil)
	so.bs.SetPointer(&alpha_cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Root: &sobject.SORoot{
			Inner:      []byte("not a plaintext SORootInner"),
			InnerSeqno: 1,
		},
		Packs: []*packfile.PackfileEntry{{Id: "01PACKA"}},
	})

	if _, err := so.GetHeadInnerState(); err == nil {
		t.Fatal("expected undecodable packed CDN root to fail")
	}
}

// TestRefreshSnapshotEmitsOnWatch covers the Phase H iter 2 contract: after
// RefreshSnapshot() succeeds, the watchable returned by AccessSharedObjectState
// surfaces a fresh cdnStateSnapshot pointer so downstream consumers (engine
// refresh loop, session resource) can react to CDN root changes.
func TestRefreshSnapshotEmitsOnWatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Stub CDN server serves a minimal CdnRootPointer on every fetch. The
	// contents don't matter; we only need Refresh() to reach the
	// s.watch.SetValue() call inside RefreshSnapshot.
	ptr := &alpha_cdn.CdnRootPointer{SpaceId: testSpaceID}
	ptrBytes, err := ptr.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	encoded := []byte(packedmsg.EncodePackedMessage(ptrBytes))

	mux := http.NewServeMux()
	mux.HandleFunc("/"+testSpaceID+"/root.packedmsg", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(encoded)
	})
	hs := httptest.NewServer(mux)
	defer hs.Close()

	bs, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	so, err := NewCdnSharedObject(CdnSharedObjectOptions{
		SpaceID:    testSpaceID,
		BlockStore: bs,
	})
	if err != nil {
		t.Fatal(err)
	}

	watch, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	initial := watch.GetValue()
	if initial == nil {
		t.Fatal("expected non-nil initial snapshot")
	}

	if err := so.RefreshSnapshot(ctx); err != nil {
		t.Fatalf("RefreshSnapshot: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	next, err := watch.WaitValueChange(waitCtx, initial, nil)
	if err != nil {
		t.Fatalf("WaitValueChange: %v", err)
	}
	if next == initial {
		t.Fatal("expected a new snapshot pointer after RefreshSnapshot")
	}
	if next == nil {
		t.Fatal("expected non-nil snapshot after RefreshSnapshot")
	}
}

func TestHealthSurfaceTracksPointerLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	so := newTestSharedObject(t, nil)

	healthCtr, rel, err := so.AccessSharedObjectHealth(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	initial := healthCtr.GetValue()
	if initial == nil {
		t.Fatal("expected initial health")
	}
	if initial.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected loading health, got %v", initial.GetStatus())
	}

	so.bs.SetPointer(&alpha_cdn.CdnRootPointer{
		SpaceId: testSpaceID,
		Root:    &sobject.SORoot{},
	})
	so.setHealth(nil)

	readyCtx, readyCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readyCancel()
	next, err := healthCtr.WaitValueChange(readyCtx, initial, nil)
	if err != nil {
		t.Fatalf("WaitValueChange() = %v", err)
	}
	if next.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatalf("expected ready health, got %v", next.GetStatus())
	}
}
