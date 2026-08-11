package manifest_fetch_world

import (
	"context"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestControllerCoalescesManifestCollection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const storeKey = "release/manifests"
	ids := []string{"core", "web", "app", "cli", "notes"}
	ws, tb := buildCollectionTestWorld(t, ctx, storeKey)
	for _, id := range ids {
		storeCollectionTestManifest(t, ctx, tb, ws, storeKey, id, "js", 2)
	}
	storeCollectionTestManifest(t, ctx, tb, ws, storeKey, ids[0], "desktop", 1)

	started := make(chan struct{})
	release := make(chan struct{})
	countedWS := &collectionCountingWorldState{
		WorldState: ws,
		seqno:      1,
		started:    started,
		release:    release,
	}
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, &Config{
		ObjectKeys:   []string{storeKey},
		DisableWatch: true,
	})
	defer func() { _ = ctrl.Close() }()

	// The leader begins sequence one and cannot return until the blocked graph
	// traversal completes. It therefore proves a later sequence re-key happens
	// before any result is published to its caller.
	type collectionResult struct {
		snapshot *manifestCollectionSnapshot
		err      error
	}
	leader := make(chan collectionResult, 1)
	go func() {
		snapshot, err := ctrl.collectManifests(ctx, countedWS)
		leader <- collectionResult{snapshot: snapshot, err: err}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Cancel a waiter while the controller-owned traversal remains available to
	// the leader and other resolvers.
	cancelCtx, cancelCollection := context.WithCancel(ctx)
	canceled := make(chan error, 1)
	go func() {
		_, err := ctrl.collectManifests(cancelCtx, countedWS)
		canceled <- err
	}()
	cancelCollection()
	select {
	case err := <-canceled:
		if !stderrors.Is(err, context.Canceled) {
			t.Fatalf("canceled collection error = %v, want context canceled", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Resolve five IDs concurrently through the same controller and world state.
	type result struct {
		id      string
		handler *collectionTestHandler
		err     error
	}
	results := make(chan result, len(ids))
	for _, id := range ids {
		go func() {
			handler := &collectionTestHandler{}
			resolver := &fetchManifestResolver{
				c:   ctrl,
				dir: manifest.NewFetchManifest(id, nil, []string{"js"}, 0),
			}
			_, err := resolver.reconcileManifestsCore(ctx, logrus.NewEntry(logrus.New()), handler, countedWS)
			results <- result{id: id, handler: handler, err: err}
		}()
	}

	// Advance the world while the first traversal is blocked. No resolver may
	// publish that sequence-one map; they must coalesce again at sequence two.
	countedWS.setSeqno(2)
	close(release)
	select {
	case result := <-leader:
		if result.err != nil {
			t.Fatalf("sequence-advanced manifest collection: %v", result.err)
		}
		if result.snapshot == nil || result.snapshot.key.seqno != 2 {
			t.Fatalf("leader manifest snapshot = %#v, want sequence 2", result.snapshot)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for range ids {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("concurrent FetchManifest %s: %v", result.id, result.err)
			}
			assertCollectionTestManifest(t, result.handler, result.id, "js", 2)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if got := countedWS.traversals(); got != 2 {
		t.Fatalf("manifest graph traversals across sequence advance = %d, want 2", got)
	}
	ctrl.mtx.Lock()
	snapshot := ctrl.collectionSnapshot
	ctrl.mtx.Unlock()
	if snapshot == nil || snapshot.key.seqno != 2 {
		t.Fatalf("published manifest snapshot = %#v, want sequence 2", snapshot)
	}

	// Reuse the same immutable snapshot for sequential IDs.
	for _, id := range ids {
		handler := &collectionTestHandler{}
		resolver := &fetchManifestResolver{
			c:   ctrl,
			dir: manifest.NewFetchManifest(id, nil, []string{"js"}, 0),
		}
		if _, err := resolver.reconcileManifestsCore(ctx, logrus.NewEntry(logrus.New()), handler, countedWS); err != nil {
			t.Fatalf("sequential FetchManifest %s: %v", id, err)
		}
		assertCollectionTestManifest(t, handler, id, "js", 2)
	}
	if got := countedWS.traversals(); got != 2 {
		t.Fatalf("sequential manifest graph traversals = %d, want 2", got)
	}

	// A world sequence advance invalidates the snapshot and traverses again.
	countedWS.setSeqno(3)
	handler := &collectionTestHandler{}
	resolver := &fetchManifestResolver{
		c:   ctrl,
		dir: manifest.NewFetchManifest(ids[0], nil, []string{"js"}, 0),
	}
	if _, err := resolver.reconcileManifestsCore(ctx, logrus.NewEntry(logrus.New()), handler, countedWS); err != nil {
		t.Fatalf("FetchManifest after sequence advance: %v", err)
	}
	assertCollectionTestManifest(t, handler, ids[0], "js", 2)
	if got := countedWS.traversals(); got != 3 {
		t.Fatalf("manifest graph traversals after sequence advance = %d, want 3", got)
	}
}

func TestControllerRekeysUnsupportedHashReset(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const storeKey = "release/manifests"
	const badManifestKey = "release/manifests/bad"
	ws, _ := buildCollectionTestWorld(t, ctx, storeKey)
	badRef := &bucket.ObjectRef{
		RootRef: block.NewBlockRef(hash.NewHash(hash.HashType(999), []byte{1, 2, 3})),
	}
	if _, err := ws.CreateObject(ctx, badManifestKey, badRef); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, badManifestKey, bldr_manifest_world.ManifestTypeID); err != nil {
		t.Fatal(err)
	}
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(storeKey, badManifestKey, "core")); err != nil {
		t.Fatal(err)
	}

	countedWS := &collectionCountingWorldState{
		WorldState:       ws,
		seqno:            1,
		seqnoAfterDelete: 2,
	}
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, &Config{ObjectKeys: []string{storeKey}})
	defer func() { _ = ctrl.Close() }()

	snapshot, err := ctrl.collectManifests(ctx, countedWS)
	if err != nil {
		t.Fatalf("collect manifests after unsupported hash reset: %v", err)
	}
	if snapshot.key.seqno != 2 || len(snapshot.manifests) != 0 {
		t.Fatalf("manifest snapshot after reset = %#v, want empty sequence 2", snapshot)
	}
	// Reset traverses candidates once, then the sequence-two collection traverses
	// the empty replacement store once more.
	if got := countedWS.traversals(); got != 3 {
		t.Fatalf("manifest graph traversals across reset = %d, want 3", got)
	}
	if err := bldr_manifest_world.CheckManifestStoreType(ctx, ws, storeKey); err != nil {
		t.Fatalf("reset manifest store: %v", err)
	}

	if _, err := ctrl.collectManifests(ctx, countedWS); err != nil {
		t.Fatalf("cached post-reset manifest collection: %v", err)
	}
	if got := countedWS.traversals(); got != 3 {
		t.Fatalf("post-reset manifest graph traversals = %d, want 3", got)
	}
}

func TestControllerRetriesFailedAndCanceledManifestCollections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const storeKey = "release/manifests"
	ws, _ := buildCollectionTestWorld(t, ctx, storeKey)
	countedWS := &collectionCountingWorldState{
		WorldState:   ws,
		seqno:        1,
		traversalErr: stderrors.New("manifest graph unavailable"),
	}
	ctrl := NewController(logrus.NewEntry(logrus.New()), nil, &Config{ObjectKeys: []string{storeKey}})
	defer func() { _ = ctrl.Close() }()

	if _, err := ctrl.collectManifests(ctx, countedWS); !stderrors.Is(err, countedWS.getTraversalErr()) {
		t.Fatalf("failed collection error = %v, want graph error", err)
	}
	countedWS.setTraversalErr(context.Canceled)
	if _, err := ctrl.collectManifests(ctx, countedWS); !stderrors.Is(err, context.Canceled) {
		t.Fatalf("canceled collection error = %v, want context canceled", err)
	}

	countedWS.setTraversalErr(nil)
	if _, err := ctrl.collectManifests(ctx, countedWS); err != nil {
		t.Fatalf("retry manifest collection: %v", err)
	}
	if _, err := ctrl.collectManifests(ctx, countedWS); err != nil {
		t.Fatalf("cached manifest collection: %v", err)
	}
	if got := countedWS.traversals(); got != 3 {
		t.Fatalf("manifest graph traversals after retries = %d, want 3", got)
	}
	if err := ctrl.Close(); err != nil {
		t.Fatalf("close manifest collection controller: %v", err)
	}
	ctrl.mtx.Lock()
	snapshot := ctrl.collectionSnapshot
	ctrl.mtx.Unlock()
	if snapshot != nil {
		t.Fatal("controller close retained manifest collection snapshot")
	}
}

func buildCollectionTestWorld(
	t *testing.T,
	ctx context.Context,
	storeKey string,
) (world.WorldState, *testbed.Testbed) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ocs.Release)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err)
	}
	return ws, tb
}

func storeCollectionTestManifest(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	ws world.WorldState,
	storeKey string,
	manifestID string,
	platformID string,
	rev uint64,
) {
	t.Helper()
	meta := &manifest.ManifestMeta{
		ManifestId: manifestID,
		BuildType:  "production",
		PlatformId: platformID,
		Rev:        rev,
	}
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Release()

	tx, bcs := cursor.BuildTransaction(nil)
	bcs.SetBlock(manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	cursor.SetRootRef(rootRef)
	manifestRef := manifest.NewManifestRef(meta, cursor.GetRef())
	manifestKey := manifest.NewManifestKey(storeKey, meta)
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx,
		ws,
		peer.ID("test"),
		manifestKey,
		[]string{storeKey},
		manifestRef,
	); err != nil {
		t.Fatal(err)
	}
}

func assertCollectionTestManifest(
	t *testing.T,
	handler *collectionTestHandler,
	manifestID string,
	platformID string,
	rev uint64,
) {
	t.Helper()
	if len(handler.values) != 1 {
		t.Fatalf("FetchManifest %s values = %d, want 1", manifestID, len(handler.values))
	}
	value, ok := handler.values[0].(*manifest.FetchManifestValue)
	if !ok {
		t.Fatalf("FetchManifest %s value type = %T", manifestID, handler.values[0])
	}
	refs := value.GetManifestRefs()
	if len(refs) != 1 {
		t.Fatalf("FetchManifest %s refs = %d, want 1", manifestID, len(refs))
	}
	meta := refs[0].GetMeta()
	if meta.GetManifestId() != manifestID || meta.GetPlatformId() != platformID || meta.GetRev() != rev {
		t.Fatalf("FetchManifest %s metadata = %#v", manifestID, meta)
	}
}

type collectionCountingWorldState struct {
	world.WorldState

	mtx              sync.Mutex
	seqno            uint64
	traversalCount   int
	traversalErr     error
	seqnoAfterDelete uint64
	started          chan struct{}
	startedOnce      sync.Once
	release          <-chan struct{}
}

func (w *collectionCountingWorldState) GetSeqno(context.Context) (uint64, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.seqno, nil
}

func (w *collectionCountingWorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	deleted, err := w.WorldState.DeleteObject(ctx, key)
	if err == nil && w.seqnoAfterDelete != 0 {
		w.setSeqno(w.seqnoAfterDelete)
	}
	return deleted, err
}

func (w *collectionCountingWorldState) AccessCayleyGraph(
	ctx context.Context,
	write bool,
	cb func(context.Context, world.CayleyHandle) error,
) error {
	w.mtx.Lock()
	w.traversalCount++
	traversalErr := w.traversalErr
	release := w.release
	started := w.started
	w.mtx.Unlock()

	if started != nil {
		w.startedOnce.Do(func() { close(started) })
	}
	if release != nil {
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-release:
		}
	}
	if traversalErr != nil {
		return traversalErr
	}
	return w.WorldState.AccessCayleyGraph(ctx, write, cb)
}

func (w *collectionCountingWorldState) setSeqno(seqno uint64) {
	w.mtx.Lock()
	w.seqno = seqno
	w.mtx.Unlock()
}

func (w *collectionCountingWorldState) setTraversalErr(err error) {
	w.mtx.Lock()
	w.traversalErr = err
	w.mtx.Unlock()
}

func (w *collectionCountingWorldState) getTraversalErr() error {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.traversalErr
}

func (w *collectionCountingWorldState) traversals() int {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.traversalCount
}

type collectionTestHandler struct {
	values []directive.Value
	idle   bool
}

func (h *collectionTestHandler) AddValue(value directive.Value) (uint32, bool) {
	h.values = append(h.values, value)
	return uint32(len(h.values)), true
}

func (h *collectionTestHandler) RemoveValue(id uint32) (directive.Value, bool) {
	if id == 0 || int(id) > len(h.values) {
		return nil, false
	}
	value := h.values[id-1]
	h.values[id-1] = nil
	return value, true
}

func (h *collectionTestHandler) CountValues(allResolvers bool) int {
	return len(h.values)
}

func (h *collectionTestHandler) ClearValues() []uint32 {
	ids := make([]uint32, len(h.values))
	for i := range h.values {
		ids[i] = uint32(i + 1)
	}
	h.values = nil
	return ids
}

func (h *collectionTestHandler) MarkIdle(idle bool) {
	h.idle = idle
}

func (h *collectionTestHandler) AddValueRemovedCallback(uint32, func()) func() {
	return func() {}
}

func (h *collectionTestHandler) AddResolverRemovedCallback(func()) func() {
	return func() {}
}

func (h *collectionTestHandler) AddResolver(directive.Resolver, func()) func() {
	return func() {}
}

// _ is a type assertion
var _ directive.ResolverHandler = (*collectionTestHandler)(nil)

// _ is a type assertion
var _ world.WorldState = (*collectionCountingWorldState)(nil)
