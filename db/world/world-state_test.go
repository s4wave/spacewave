package world_test

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
)

func TestAccessObjectReturnsStorageOpArgs(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Build the object body through a stage.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	stage, err := ws.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Release()
	ref, err := world.AccessObject(ctx, stage.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("root"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref.GetBucketId() != wtb.EngineBucketID {
		t.Fatalf("expected bucket id %q, got %q", wtb.EngineBucketID, ref.GetBucketId())
	}
	if ref.GetTransformConf().GetEmpty() {
		t.Fatal("expected object ref to retain the storage transform config")
	}

	// Read the saved object root and verify its body.
	example, err := world.LookupObjectRef[block_mock.Example](ctx, ws.AccessWorldState, ref)
	if err != nil {
		t.Fatal(err)
	}
	if example.GetMsg() != "root" {
		t.Fatalf("expected root block message, got %q", example.GetMsg())
	}
}

func TestLookupObjectBodyReleasesObjectState(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, "example/body-release", func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("body"), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Observe object state releases while reading or accessing its body.
	wrapped := &releaseCountingWorldState{WorldState: ws}
	example, err := world.LookupObjectBody[block_mock.Example](
		ctx,
		wrapped,
		"example/body-release",
	)
	if err != nil {
		t.Fatal(err)
	}
	if example.GetMsg() != "body" {
		t.Fatalf("expected body message, got %q", example.GetMsg())
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected lookup to release object state once, got %d", wrapped.releases)
	}
}

func TestLookupObjectBodiesPreservesKeysAndMissingMarkers(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	for _, entry := range []struct {
		key string
		msg string
	}{
		{key: "example/batch-alpha", msg: "alpha"},
		{key: "example/batch-beta", msg: "beta"},
		{key: "example/batch-gamma", msg: "gamma"},
	} {
		// Save the example body under its requested object key.
		var createdObject world.ObjectState
		createdObject, _, err = world.CreateWorldObject(ctx, ws, entry.key, func(bcs *block.Cursor) error {
			bcs.SetBlock(block_mock.NewExample(entry.msg), true)
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Read object bodies in an order containing missing and repeated keys.
	results, err := world.LookupObjectBodies[block_mock.Example](
		ctx,
		ws,
		[]string{
			"example/batch-gamma",
			"example/batch-missing",
			"example/batch-alpha",
			"example/batch-alpha",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Check the key, existence marker, and body of every batch result.
	check := func(index int, key string, msg string, exists bool) {
		// Verify that the returned body matches its requested object key.
		t.Helper()
		result := results[index]
		if result.ObjectKey != key || result.Exists != exists {
			t.Fatalf("result %d = key %q exists %v, want key %q exists %v", index, result.ObjectKey, result.Exists, key, exists)
		}
		if !exists {
			if result.Body != nil {
				t.Fatalf("result %d has body for missing key", index)
			}
			return
		}
		if result.Body == nil || result.Body.GetMsg() != msg {
			t.Fatalf("result %d body = %v, want %q", index, result.Body, msg)
		}
	}

	// Assert body order, missing markers, and repeated key results.
	check(0, "example/batch-gamma", "gamma", true)
	check(1, "example/batch-missing", "", false)
	check(2, "example/batch-alpha", "alpha", true)
	check(3, "example/batch-alpha", "alpha", true)
}

func TestAccessWorldObjectReleasesObjectState(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/access-release"
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("access"), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Observe object state releases while reading or accessing its body.
	wrapped := &releaseCountingWorldState{WorldState: ws}
	if _, _, err := world.AccessWorldObject(ctx, wrapped, key, false, func(*block.Cursor) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected existing-object success to release once, got %d", wrapped.releases)
	}

	// Check that callback failure releases the existing object state.
	wrapped.releases = 0
	callbackErr := errors.New("callback failed")
	if _, _, err := world.AccessWorldObject(ctx, wrapped, key, false, func(*block.Cursor) error {
		return callbackErr
	}); err != callbackErr {
		t.Fatalf("expected callback error, got %v", err)
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected existing-object error to release once, got %d", wrapped.releases)
	}

	// Check that reading a missing object acquires no object state.
	wrapped.releases = 0
	if _, _, err := world.AccessWorldObject(ctx, wrapped, "example/access-missing", false, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("missing"), true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if wrapped.releases != 0 {
		t.Fatalf("expected not-found access to release no state, got %d", wrapped.releases)
	}

	// Check that creating a missing object releases its returned state.
	wrapped.releases = 0
	if _, _, err := world.AccessWorldObject(ctx, wrapped, "example/access-created", true, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("created"), true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected created-object state to release once, got %d", wrapped.releases)
	}
}

func TestCreateWorldObjectReleasesExistingObjectState(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/create-release"
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("create"), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Observe object state releases while reading or accessing its body.
	wrapped := &releaseCountingWorldState{WorldState: ws}
	var createdObject2 world.ObjectState
	createdObject2, _, err = world.CreateWorldObject(ctx, wrapped, key, func(*block.Cursor) error {
		return nil
	})
	world.ReleaseObjectState(createdObject2)
	if err != world.ErrObjectExists {
		t.Fatalf("expected object-exists error, got %v", err)
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected existing-object check to release once, got %d", wrapped.releases)
	}

	// Check that creating a missing object acquires no state during lookup.
	wrapped.releases = 0
	var createdObject3 world.ObjectState
	createdObject3, _, err = world.CreateWorldObject(ctx, wrapped, "example/create-missing", func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("missing"), true)
		return nil
	})
	defer world.ReleaseObjectState(createdObject3)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.releases != 0 {
		t.Fatalf("expected not-found create check to release no state, got %d", wrapped.releases)
	}
}

func TestLookupRootRefReleasesObjectState(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/root-ref-release"
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("root-ref"), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Count object state releases while reading an existing object root.
	var releases int
	eng := &releaseCountingEngine{Engine: wtb.Engine, releases: &releases}
	if _, _, err := world.LookupRootRef(ctx, eng, key); err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("expected root-ref lookup to release once, got %d", releases)
	}

	// Check that an absent object root acquires no object state.
	releases = 0
	if _, _, err := world.LookupRootRef(ctx, eng, "example/root-ref-missing"); err != nil {
		t.Fatal(err)
	}
	if releases != 0 {
		t.Fatalf("expected not-found root-ref lookup to release no state, got %d", releases)
	}
}

func TestLookupObjectReleasesDecodeErrorState(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/decode-release"
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(&releaseTestBlock{data: "bad"}, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Observe object state releases while reading or accessing its body.
	wrapped := &releaseCountingWorldState{WorldState: ws}
	value, obj, err := world.LookupObject[releaseTestBlock](
		ctx,
		wrapped,
		key,
	)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if value != nil {
		t.Fatalf("expected no decoded value, got %#v", value)
	}
	if obj != nil {
		t.Fatal("expected decode-error object state to be unavailable")
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected decode error to release once, got %d", wrapped.releases)
	}
}

func TestCollectObjectBodiesReleasesPartialFailureStates(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	for key, data := range map[string]string{
		"example/collect-good": "good",
		"example/collect-bad":  "bad",
	} {
		// Save a body that either decodes successfully or fails decoding.
		var createdObject world.ObjectState
		createdObject, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
			bcs.SetBlock(&releaseTestBlock{data: data}, true)
			return nil
		})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Observe object state releases while reading or accessing its body.
	wrapped := &releaseCountingWorldState{WorldState: ws}
	values, states, err := world.CollectObjectBodies[releaseTestBlock](
		ctx,
		wrapped,
		[]string{"example/collect-good", "example/collect-bad"},
	)
	if err == nil {
		t.Fatal("expected partial collection decode error")
	}
	if values != nil || states != nil {
		t.Fatalf("expected fatal collection to return nil slices, got %#v %#v", values, states)
	}
	if wrapped.releases != 2 {
		t.Fatalf("expected partial collection to release both states once, got %d", wrapped.releases)
	}
}

func TestEngineWorldStateRetriesStaleGenerationWriteOperation(t *testing.T) {
	// Cover stale generation failures at each write transaction stage.
	cases := []struct {
		name      string
		configure func(*staleRetryEngine)
	}{
		{
			name: "transaction creation",
			configure: func(eng *staleRetryEngine) {
				eng.staleNewTx = 1
			},
		},
		{
			name: "operation body",
			configure: func(eng *staleRetryEngine) {
				eng.staleCreate = 1
			},
		},
		{
			name: "commit",
			configure: func(eng *staleRetryEngine) {
				eng.commitRejections = []error{coord.ErrStaleGeneration}
			},
		},
	}

	// Verify that each transient stale failure permits a committed object.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Configure an engine that fails once at the selected transaction stage.
			ctx := context.Background()
			eng := &staleRetryEngine{objects: make(map[string]*bucket.ObjectRef)}
			tc.configure(eng)
			ws := world.NewEngineWorldState(eng, true)
			key := "retry-object"

			// Create the object through the engine retry path.
			createdObject, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "bucket"})
			world.ReleaseObjectState(createdObject)
			if err != nil {
				t.Fatalf("CreateObject returned error after transient stale generation: %v", err)
			}

			// Verify that the retried creation committed the object.
			found, err := ws.HasObject(ctx, key)
			if err != nil {
				t.Fatalf("HasObject after retried create returned error: %v", err)
			}
			if !found {
				t.Fatal("object created after transient stale generation was not committed")
			}
		})
	}
}

// TestExecTransactionReplaysStaleCommit checks that a transaction replays
// rejected commits, but not a commit whose write may already be durable.
func TestExecTransactionReplaysStaleCommit(t *testing.T) {
	// Distinguish rejections before publication from an error after publication.
	cases := []struct {
		// name identifies the commit scenario.
		name string
		// rejections fail an attempt before it publishes.
		rejections []error
		// outcome is reported after publishing the object.
		outcome error
		// attempts counts the expected callback applications.
		attempts int
	}{
		{
			name:       "stale generations",
			rejections: []error{coord.ErrStaleGeneration, coord.ErrStaleGeneration},
			attempts:   3,
		},
		{
			name:       "invalid snapshot",
			rejections: []error{kvtx.ErrInvalidSnapshot},
			attempts:   2,
		},
		{
			name:     "unknown despite invalid snapshot",
			outcome:  errors.Join(kvtx.ErrInvalidSnapshot, world.ErrDurabilityUnknown),
			attempts: 1,
		},
	}

	// Exercise the full transaction loop for each commit scenario.
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Create one object, counting callback applications across attempts.
			eng := &staleRetryEngine{
				commitRejections: tc.rejections,
				commitOutcome:    tc.outcome,
				objects:          make(map[string]*bucket.ObjectRef),
			}
			const key = "retry-object"
			attempts := 0
			err := world.ExecTransaction(t.Context(), eng, true, func(ctx context.Context, ws world.WorldState) error {
				attempts++
				createdObject, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "bucket"})
				world.ReleaseObjectState(createdObject)
				return err
			})

			// Check that only rejections replay and that the published object survives.
			if attempts != tc.attempts {
				t.Errorf("callback attempts = %d, want %d", attempts, tc.attempts)
			}
			if !errors.Is(err, tc.outcome) {
				t.Errorf("ExecTransaction error = %v, want %v", err, tc.outcome)
			}
			if _, found := eng.objects[key]; !found {
				t.Fatal("published object not found")
			}
		})
	}
}

// TestIsSupersededExcludesUnknownOutcome checks that an unknown outcome is
// never read as superseded, so no retrier replays a write that may be durable.
func TestIsSupersededExcludesUnknownOutcome(t *testing.T) {
	for _, refusal := range []error{world.ErrSuperseded, coord.ErrStaleGeneration} {
		if !world.IsSuperseded(refusal) {
			t.Fatalf("IsSuperseded(%v) = false, want true", refusal)
		}
		if err := errors.Join(world.ErrDurabilityUnknown, refusal); world.IsSuperseded(err) {
			t.Fatalf("IsSuperseded(%v) = true, want false", err)
		}
	}
}

type releaseCountingWorldState struct {
	world.WorldState
	releases int
}

func (ws *releaseCountingWorldState) GetObject(
	ctx context.Context,
	key string,
) (world.ObjectState, bool, error) {
	obj, found, err := ws.WorldState.GetObject(ctx, key)
	if err != nil || !found {
		return obj, found, err
	}
	return &releaseCountingObjectState{
		ObjectState: obj,
		releases:    &ws.releases,
	}, true, nil
}

// GetObjectBodiesBatchPage reads each body through the counted GetObject, as
// a store without a batch read does.
func (ws *releaseCountingWorldState) GetObjectBodiesBatchPage(
	ctx context.Context,
	keys []string,
	byteBudget int,
) ([]*world.ObjectBody, uint32, uint64, error) {
	return world.LookupObjectBodiesPage(ctx, ws, keys, byteBudget, 1)
}

func (ws *releaseCountingWorldState) CreateObject(
	ctx context.Context,
	key string,
	rootRef *bucket.ObjectRef,
) (world.ObjectState, error) {
	obj, err := ws.WorldState.CreateObject(ctx, key, rootRef)
	if obj == nil {
		return obj, err
	}
	return &releaseCountingObjectState{
		ObjectState: obj,
		releases:    &ws.releases,
	}, err
}

type releaseCountingObjectState struct {
	world.ObjectState
	releases *int
}

func (obj *releaseCountingObjectState) Release() {
	*obj.releases += 1
}

type releaseCountingEngine struct {
	world.Engine
	releases *int
}

func (eng *releaseCountingEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := eng.Engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &releaseCountingTx{Tx: tx, releases: eng.releases}, nil
}

type releaseCountingTx struct {
	world.Tx
	releases *int
}

func (tx *releaseCountingTx) GetObject(
	ctx context.Context,
	key string,
) (world.ObjectState, bool, error) {
	obj, found, err := tx.Tx.GetObject(ctx, key)
	if err != nil || !found {
		return obj, found, err
	}
	return &releaseCountingObjectState{
		ObjectState: obj,
		releases:    tx.releases,
	}, true, nil
}

type releaseTestBlock struct {
	data string
}

func (b *releaseTestBlock) MarshalVT() ([]byte, error) {
	return []byte(b.data), nil
}

func (b *releaseTestBlock) UnmarshalVT(data []byte) error {
	b.data = string(data)
	if b.data == "bad" {
		return errors.New("decode failed")
	}
	return nil
}

// staleRetryEngine scripts commit failures around publication for retry tests.
type staleRetryEngine struct {
	// staleNewTx counts remaining stale transaction opens.
	staleNewTx int
	// staleCreate counts remaining stale object creation attempts.
	staleCreate int
	// commitRejections holds errors to return before publishing writes.
	commitRejections []error
	// commitOutcome is a one-shot error to return after publishing writes.
	commitOutcome error
	// objects holds published objects.
	objects map[string]*bucket.ObjectRef
}

func (e *staleRetryEngine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return "", "", nil
}

func (e *staleRetryEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if e.staleNewTx > 0 {
		e.staleNewTx--
		return nil, coord.ErrStaleGeneration
	}
	return &staleRetryTx{engine: e, write: write}, nil
}

func (e *staleRetryEngine) Sync(ctx context.Context) (bool, error) {
	return false, nil
}

func (e *staleRetryEngine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	return seqno, nil
}

func (e *staleRetryEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	panic("unexpected BuildStorageCursor call")
}

func (e *staleRetryEngine) StageWorldState(context.Context) (world.WorldStage, error) {
	panic("unexpected StageWorldState call")
}

func (e *staleRetryEngine) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	panic("unexpected AccessWorldState call")
}

func (e *staleRetryEngine) GetSeqno(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (e *staleRetryEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return value, nil
}

func (e *staleRetryEngine) WatchChanges(ctx context.Context, afterSeqno uint64, _ world.ChangeFilter, cb func(uint64, *world.ChangeSet) error) error {
	return world.WatchSeqnoChanges(ctx, e, afterSeqno, cb)
}

func (e *staleRetryEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	return world.WaitObjectRevBySeqno(ctx, e, key, rev, ignoreNotFound)
}

type staleRetryTx struct {
	engine  *staleRetryEngine
	write   bool
	pending map[string]*bucket.ObjectRef
}

func (txs *staleRetryTx) GetReadOnly() bool {
	return !txs.write
}

func (txs *staleRetryTx) Sync(ctx context.Context) (bool, error) {
	return false, nil
}

func (txs *staleRetryTx) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	panic("unexpected BuildStorageCursor call")
}

func (txs *staleRetryTx) StageWorldState(context.Context) (world.WorldStage, error) {
	panic("unexpected StageWorldState call")
}

func (txs *staleRetryTx) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	panic("unexpected AccessWorldState call")
}

func (txs *staleRetryTx) ApplyWorldOp(ctx context.Context, op world.Operation, opSender peer.ID) (uint64, bool, error) {
	panic("unexpected ApplyWorldOp call")
}

func (txs *staleRetryTx) GetObjectBodiesBatchPage(ctx context.Context, keys []string, byteBudget int) ([]*world.ObjectBody, uint32, uint64, error) {
	return world.LookupObjectBodiesPage(ctx, txs, keys, byteBudget, 1)
}

func (txs *staleRetryTx) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	ref, found := txs.engine.objects[key]
	if !found {
		return nil, false, nil
	}
	return &staleRetryObject{key: key, rootRef: ref}, true, nil
}

func (txs *staleRetryTx) ListObjectsWithType(ctx context.Context, typeID, startAfter string, limit uint32) ([]string, bool, error) {
	panic("unexpected ListObjectsWithType call")
}

func (txs *staleRetryTx) ListTypeCounts(ctx context.Context) ([]*world.TypeCount, error) {
	panic("unexpected ListTypeCounts call")
}

func (txs *staleRetryTx) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	panic("unexpected IterateObjects call")
}

func (txs *staleRetryTx) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	// Reject read-only writes and simulate the requested stale creation failure.
	if !txs.write {
		return nil, tx.ErrNotWrite
	}
	if txs.engine.staleCreate > 0 {
		txs.engine.staleCreate--
		return nil, coord.ErrStaleGeneration
	}
	if txs.pending == nil {
		txs.pending = make(map[string]*bucket.ObjectRef)
	}

	// Retain the object root until this simulated transaction commits.
	txs.pending[key] = rootRef
	return &staleRetryObject{key: key, rootRef: rootRef}, nil
}

func (txs *staleRetryTx) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	panic("unexpected RenameObject call")
}

func (txs *staleRetryTx) DeleteObject(ctx context.Context, key string) (bool, error) {
	panic("unexpected DeleteObject call")
}

func (txs *staleRetryTx) HasObject(ctx context.Context, key string) (bool, error) {
	_, found := txs.engine.objects[key]
	return found, nil
}

func (txs *staleRetryTx) AccessCayleyGraph(ctx context.Context, write bool, cb func(ctx context.Context, h world.CayleyHandle) error) error {
	panic("unexpected AccessCayleyGraph call")
}

func (txs *staleRetryTx) LookupGraphQuads(ctx context.Context, filter world.GraphQuad, limit uint32) ([]world.GraphQuad, error) {
	panic("unexpected LookupGraphQuads call")
}

func (txs *staleRetryTx) LookupGraphQuadsBatch(ctx context.Context, filters []world.GraphQuad, limitPerFilter uint32) ([][]world.GraphQuad, error) {
	panic("unexpected LookupGraphQuadsBatch call")
}

func (txs *staleRetryTx) ListGraphEdgeBuckets(ctx context.Context, query *world.GraphEdgeBucketQuery) ([]*world.GraphEdgeBucket, error) {
	panic("unexpected ListGraphEdgeBuckets call")
}

func (txs *staleRetryTx) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	panic("unexpected QueryGraphPath call")
}

func (txs *staleRetryTx) SetGraphQuad(ctx context.Context, q world.GraphQuad) error {
	panic("unexpected SetGraphQuad call")
}

func (txs *staleRetryTx) DeleteGraphQuad(ctx context.Context, q world.GraphQuad) error {
	panic("unexpected DeleteGraphQuad call")
}

func (txs *staleRetryTx) DeleteGraphObject(ctx context.Context, value string) error {
	panic("unexpected DeleteGraphObject call")
}

func (txs *staleRetryTx) GetSeqno(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (txs *staleRetryTx) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	return value, nil
}

// Commit rejects a queued attempt or publishes its writes before reporting an outcome.
func (txs *staleRetryTx) Commit(ctx context.Context) error {
	// Return the next rejection without publishing this attempt's writes.
	if len(txs.engine.commitRejections) != 0 {
		err := txs.engine.commitRejections[0]
		txs.engine.commitRejections = txs.engine.commitRejections[1:]
		return err
	}

	// Publish the writes even if the outcome reported to the caller is unknown.
	maps.Copy(txs.engine.objects, txs.pending)
	err := txs.engine.commitOutcome
	txs.engine.commitOutcome = nil
	return err
}

func (txs *staleRetryTx) Discard() {}

type staleRetryObject struct {
	key     string
	rootRef *bucket.ObjectRef
}

func (obj *staleRetryObject) GetModSeqno(context.Context) (uint64, error) {
	return 0, nil
}

func (obj *staleRetryObject) GetKey() string {
	return obj.key
}

func (obj *staleRetryObject) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	return obj.rootRef, 1, nil
}

func (obj *staleRetryObject) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	panic("unexpected object AccessWorldState call")
}

func (obj *staleRetryObject) SetRootRef(ctx context.Context, nref *bucket.ObjectRef) (uint64, error) {
	panic("unexpected SetRootRef call")
}

func (obj *staleRetryObject) ApplyObjectOp(ctx context.Context, op world.Operation, opSender peer.ID) (uint64, bool, error) {
	panic("unexpected ApplyObjectOp call")
}

func (obj *staleRetryObject) IncrementRev(ctx context.Context) (uint64, error) {
	panic("unexpected IncrementRev call")
}

func (obj *staleRetryObject) WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (uint64, error) {
	panic("unexpected WaitRev call")
}

// TestAccessObjectStateKeepsConcurrentUpdate publishes an engine object update
// that another writer changed mid-access onto that writer's root instead of
// overwriting it.
func TestAccessObjectStateKeepsConcurrentUpdate(t *testing.T) {
	// Open a World testbed for object storage and release checks.
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Populate the World with the object bodies used by this test.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/concurrent-update"
	created, _, err := world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample(""), true)
		return nil
	})
	world.ReleaseObjectState(created)
	if err != nil {
		t.Fatal(err)
	}

	// appendMsg returns a callback that appends suffix to the example message.
	appendMsg := func(suffix string) world.AccessObjectCb {
		return func(bcs *block.Cursor) error {
			ex, err := block_mock.UnmarshalExample(ctx, bcs)
			if err != nil {
				return err
			}
			bcs.SetBlock(block_mock.NewExample(ex.GetMsg()+suffix), true)
			return nil
		}
	}

	// Writer b commits while writer a's first attempt is still running.
	var attempts int
	appendA := appendMsg("a")
	if _, _, err := world.AccessWorldObject(ctx, ws, key, true, func(bcs *block.Cursor) error {
		attempts++
		if attempts == 1 {
			if _, _, err := world.AccessWorldObject(ctx, ws, key, true, appendMsg("b")); err != nil {
				return err
			}
		}
		return appendA(bcs)
	}); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("callback ran %d times, want 2", attempts)
	}

	// Read the final object body and verify both writer updates survived.
	var got string
	if _, _, err := world.AccessWorldObject(ctx, ws, key, false, func(bcs *block.Cursor) error {
		ex, err := block_mock.UnmarshalExample(ctx, bcs)
		got = ex.GetMsg()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got != "ba" {
		t.Fatalf("object message = %q, want %q", got, "ba")
	}
}
