package cdn_sharedobject

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	alpha_cdn "github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// testSpaceID identifies the isolated CDN fixture.
const testSpaceID = "01kpftest0000000000000001"

// testCheckpoint returns a genesis checkpoint of the fixture holding the plain
// World state data.
func testCheckpoint(t *testing.T, stateData []byte) *sobject.SOCheckpoint {
	t.Helper()
	inner, err := (&sobject.SOCheckpointInner{
		SharedObjectId: testSpaceID,
		ConfigHash:     make([]byte, sha256.Size),
		ReplayVersion:  sobject.SOReplayVersion,
		StateData:      stateData,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return &sobject.SOCheckpoint{Inner: inner}
}

// testHeadCheckpoint returns a checkpoint whose World has an empty head ref.
func testHeadCheckpoint(t *testing.T) *sobject.SOCheckpoint {
	t.Helper()
	data, err := (&sobject_world_engine.InnerState{HeadRef: &bucket.ObjectRef{}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return testCheckpoint(t, data)
}

// newTestSharedObject builds a CdnSharedObject wrapped around a CdnBlockStore
// whose pointer holds seed, when set. The block store's network side is
// unused because the test touches only the metadata and snapshot surface.
func newTestSharedObject(t *testing.T, seed *sobject.SOCheckpoint) *CdnSharedObject {
	// Build the CDN block store, seed its pointer, and wrap it.
	t.Helper()
	bs, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: "https://example.invalid",
		SpaceID:    testSpaceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bs.Close)
	if seed != nil {
		bs.SetPointer(&alpha_cdn.CdnRootPointer{
			SpaceId:    testSpaceID,
			Checkpoint: seed,
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

// setTestPointer publishes a root pointer on the shared object's CDN block
// store. The tests build the shared object over a CdnBlockStore, which owns
// the pointer; SuppliedBlockStore mounts read the pointer from the CDN.
func setTestPointer(t *testing.T, so *CdnSharedObject, ptr *alpha_cdn.CdnRootPointer) {
	t.Helper()
	bs, ok := so.bs.(*cdn_bstore.CdnBlockStore)
	if !ok {
		t.Fatalf("test shared object block store = %T", so.bs)
	}
	bs.SetPointer(ptr)
}

// TestMetadataSurface exposes CDN identity and public-read metadata.
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

// TestWritePathsRejected rejects every SharedObject mutation surface.
func TestWritePathsRejected(t *testing.T) {
	ctx := context.Background()
	so := newTestSharedObject(t, nil)
	if _, err := so.QueueOperation(ctx, []byte("x")); err == nil {
		t.Fatal("expected QueueOperation to error")
	}
	if _, _, err := so.AccessLocalStateStore(ctx, "state", nil); err == nil {
		t.Fatal("expected AccessLocalStateStore to error")
	}
}

// TestSnapshotBeforeAndAfterPointer distinguishes an absent head from a decoded published head.
func TestSnapshotBeforeAndAfterPointer(t *testing.T) {
	// Before any pointer is cached, the snapshot holds no checkpoint and no
	// operations, so callers can tell a fresh Space from a decode error.
	ctx := context.Background()
	so := newTestSharedObject(t, nil)
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// It holds no operations.
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if set.Len() != 0 {
		t.Fatalf("expected no operations, got %d", set.Len())
	}

	// It holds no checkpoint.
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint != nil {
		t.Fatalf("expected no checkpoint before the pointer is cached, got %+v", checkpoint)
	}

	// After a pointer is cached, the snapshot decodes the plain checkpoint
	// and its World head.
	published := testHeadCheckpoint(t)
	setTestPointer(t, so, &alpha_cdn.CdnRootPointer{SpaceId: testSpaceID, Checkpoint: published})
	checkpoint, err = snap.GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := published.UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	if !checkpoint.EqualVT(want) {
		t.Fatalf("decoded checkpoint mismatch: %+v", checkpoint)
	}

	// The published World head is readable.
	head, err := so.GetHeadInnerState()
	if err != nil {
		t.Fatal(err)
	}
	if head.GetHeadRef() == nil {
		t.Fatal("expected the published World head")
	}
}

// TestEmptyInitializedPointerHasNoHead treats a genesis checkpoint with no
// World as loading.
func TestEmptyInitializedPointerHasNoHead(t *testing.T) {
	so := newTestSharedObject(t, testCheckpoint(t, nil))
	head, err := so.GetHeadInnerState()
	if err != nil {
		t.Fatal(err)
	}
	if head.GetHeadRef() != nil {
		t.Fatalf("head = %+v, want none for an empty genesis checkpoint", head)
	}
}

// TestWorldEngineMissingPublishedHeadReturnsSharedObjectLoadingHealth reports retryable loading health before publication.
func TestWorldEngineMissingPublishedHeadReturnsSharedObjectLoadingHealth(t *testing.T) {
	// Publish a genesis checkpoint with no World.
	ctx := context.Background()
	ptr := &alpha_cdn.CdnRootPointer{
		SpaceId:    testSpaceID,
		Checkpoint: testCheckpoint(t, nil),
	}
	ptrBytes, err := ptr.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	encoded := []byte(packedmsg.EncodePackedMessage(ptrBytes))

	// Serve the pointer from a test CDN.
	mux := http.NewServeMux()
	mux.HandleFunc("/"+testSpaceID+"/root.packedmsg", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(encoded)
	})
	hs := httptest.NewServer(mux)
	t.Cleanup(hs.Close)

	// Build the shared object over the test CDN.
	bs, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bs.Close)
	so, err := NewCdnSharedObject(CdnSharedObjectOptions{
		SpaceID:    testSpaceID,
		BlockStore: bs,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Building a World engine reports loading health on the shared object layer.
	_, err = NewWorldEngine(ctx, logrus.NewEntry(logrus.New()), nil, so)
	if err == nil {
		t.Fatal("expected missing published head to block world engine construction")
	}
	health, ok := sobject.GetSharedObjectHealthFromError(err)
	if !ok {
		t.Fatalf("expected typed SharedObject health, got %v", err)
	}
	if health.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected loading health, got %v", health.GetStatus())
	}
	if health.GetLayer() != sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT {
		t.Fatalf("expected shared-object layer, got %v", health.GetLayer())
	}
}

// TestWorldEngineFollowsRefsThroughCdnBucket resolves authoring bucket refs
// through the CDN Space bucket.
func TestWorldEngineFollowsRefsThroughCdnBucket(t *testing.T) {
	// Publish a World head.
	ctx := context.Background()
	so := newTestSharedObject(t, testHeadCheckpoint(t))

	// Build a World engine and follow an authoring ref into the CDN bucket.
	le := logrus.NewEntry(logrus.New())
	first, err := NewWorldEngine(ctx, le, nil, so)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Release)
	followed, err := first.Cursor.FollowRef(ctx, &bucket.ObjectRef{BucketId: "authoring-world"})
	if err != nil {
		t.Fatalf("follow authoring ref through CDN bucket override: %v", err)
	}
	if got := followed.GetOpArgs().GetBucketId(); got != testSpaceID {
		followed.Release()
		t.Fatalf("followed bucket = %q, want %q", got, testSpaceID)
	}
	followed.Release()
}

// TestPackedPointerRejectsUndecodableCheckpoint rejects corrupt metadata.
func TestPackedPointerRejectsUndecodableCheckpoint(t *testing.T) {
	so := newTestSharedObject(t, nil)
	setTestPointer(t, so, &alpha_cdn.CdnRootPointer{
		SpaceId:    testSpaceID,
		Checkpoint: &sobject.SOCheckpoint{Inner: []byte("not a plaintext checkpoint")},
		Packs:      []*packfile.PackfileEntry{{Id: "01PACKA"}},
	})

	if _, err := so.GetHeadInnerState(); err == nil {
		t.Fatal("expected undecodable packed CDN root to fail")
	}
}

// TestRefreshSnapshotEmitsOnWatch verifies that RefreshSnapshot publishes a new
// snapshot through AccessSharedObjectState after a CDN root change.
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
	t.Cleanup(hs.Close)

	bs, err := cdn_bstore.NewCdnBlockStore(cdn_bstore.Options{
		CdnBaseURL: hs.URL,
		SpaceID:    testSpaceID,
		HttpClient: hs.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bs.Close)
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
	t.Cleanup(rel)

	initial := watch.GetValue()
	if initial == nil {
		t.Fatal("expected non-nil initial snapshot")
	}

	if err := so.RefreshSnapshot(ctx); err != nil {
		t.Fatalf("RefreshSnapshot: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(cancel)
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

// TestHealthSurfaceTracksPointerLifecycle publishes loading and ready health as the CDN head changes.
func TestHealthSurfaceTracksPointerLifecycle(t *testing.T) {
	// Start a shared object with no pointer.
	t.Parallel()
	ctx := context.Background()
	so := newTestSharedObject(t, nil)

	// Watch its health.
	healthCtr, rel, err := so.AccessSharedObjectHealth(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rel)

	// It starts loading.
	initial := healthCtr.GetValue()
	if initial == nil {
		t.Fatal("expected initial health")
	}
	if initial.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected loading health, got %v", initial.GetStatus())
	}

	// Publish a genesis checkpoint with no World.
	setTestPointer(t, so, &alpha_cdn.CdnRootPointer{
		SpaceId:    testSpaceID,
		Checkpoint: testCheckpoint(t, nil),
	})
	so.setHealth(nil)

	// Health stays loading while the head is missing.
	loadingCtx, loadingCancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(loadingCancel)
	next, err := healthCtr.WaitValueChange(loadingCtx, initial, nil)
	if err != nil {
		t.Fatalf("WaitValueChange() = %v", err)
	}
	if next.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected loading health for missing head, got %v", next.GetStatus())
	}

	// Publish a World head.
	setTestPointer(t, so, &alpha_cdn.CdnRootPointer{
		SpaceId:    testSpaceID,
		Checkpoint: testHeadCheckpoint(t),
	})
	so.setHealth(nil)

	// Health becomes ready.
	readyCtx, readyCancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(readyCancel)
	next, err = healthCtr.WaitValueChange(readyCtx, next, nil)
	if err != nil {
		t.Fatalf("WaitValueChange() = %v", err)
	}
	if next.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatalf("expected ready health, got %v", next.GetStatus())
	}
}
