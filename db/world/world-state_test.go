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
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
)

func TestAccessObjectReturnsStorageOpArgs(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	ref, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
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

	example, err := world.LookupObjectRef[*block_mock.Example](ctx, ws.AccessWorldState, ref, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err)
	}
	if example.GetMsg() != "root" {
		t.Fatalf("expected root block message, got %q", example.GetMsg())
	}
}

func TestLookupObjectBodyReleasesObjectState(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	_, _, err = world.CreateWorldObject(ctx, ws, "example/body-release", func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("body"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	wrapped := &releaseCountingWorldState{WorldState: ws}
	example, err := world.LookupObjectBody[*block_mock.Example](
		ctx,
		wrapped,
		"example/body-release",
		block_mock.NewExampleBlock,
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

func TestAccessWorldObjectReleasesObjectState(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/access-release"
	_, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("access"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	wrapped := &releaseCountingWorldState{WorldState: ws}
	if _, _, err := world.AccessWorldObject(ctx, wrapped, key, false, func(*block.Cursor) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected existing-object success to release once, got %d", wrapped.releases)
	}

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
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/create-release"
	_, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("create"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	wrapped := &releaseCountingWorldState{WorldState: ws}
	_, _, err = world.CreateWorldObject(ctx, wrapped, key, func(*block.Cursor) error {
		return nil
	})
	if err != world.ErrObjectExists {
		t.Fatalf("expected object-exists error, got %v", err)
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected existing-object check to release once, got %d", wrapped.releases)
	}

	wrapped.releases = 0
	_, _, err = world.CreateWorldObject(ctx, wrapped, "example/create-missing", func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("missing"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.releases != 0 {
		t.Fatalf("expected not-found create check to release no state, got %d", wrapped.releases)
	}
}

func TestLookupRootRefReleasesObjectState(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/root-ref-release"
	_, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample("root-ref"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var releases int
	eng := &releaseCountingEngine{Engine: wtb.Engine, releases: &releases}
	if _, _, err := world.LookupRootRef(ctx, eng, key); err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("expected root-ref lookup to release once, got %d", releases)
	}

	releases = 0
	if _, _, err := world.LookupRootRef(ctx, eng, "example/root-ref-missing"); err != nil {
		t.Fatal(err)
	}
	if releases != 0 {
		t.Fatalf("expected not-found root-ref lookup to release no state, got %d", releases)
	}
}

func TestLookupObjectReleasesDecodeErrorState(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	const key = "example/decode-release"
	_, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(&releaseTestBlock{data: "bad"}, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	wrapped := &releaseCountingWorldState{WorldState: ws}
	value, obj, err := world.LookupObject[*releaseTestBlock](
		ctx,
		wrapped,
		key,
		func() block.Block { return &releaseTestBlock{} },
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
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	for key, data := range map[string]string{
		"example/collect-good": "good",
		"example/collect-bad":  "bad",
	} {
		_, _, err = world.CreateWorldObject(ctx, ws, key, func(bcs *block.Cursor) error {
			bcs.SetBlock(&releaseTestBlock{data: data}, true)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	wrapped := &releaseCountingWorldState{WorldState: ws}
	values, states, err := world.CollectObjectBodies[*releaseTestBlock](
		ctx,
		wrapped,
		[]string{"example/collect-good", "example/collect-bad"},
		func() block.Block { return &releaseTestBlock{} },
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
				eng.staleCommit = 1
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			eng := &staleRetryEngine{objects: make(map[string]*bucket.ObjectRef)}
			tc.configure(eng)
			ws := world.NewEngineWorldState(eng, true)
			key := "retry-object"

			_, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "bucket"})
			if err != nil {
				t.Fatalf("CreateObject returned error after transient stale generation: %v", err)
			}
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

func (b *releaseTestBlock) MarshalBlock() ([]byte, error) {
	return []byte(b.data), nil
}

func (b *releaseTestBlock) UnmarshalBlock(data []byte) error {
	b.data = string(data)
	if b.data == "bad" {
		return errors.New("decode failed")
	}
	return nil
}

type staleRetryEngine struct {
	staleNewTx  int
	staleCreate int
	staleCommit int
	objects     map[string]*bucket.ObjectRef
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

func (e *staleRetryEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	panic("unexpected BuildStorageCursor call")
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

func (txs *staleRetryTx) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	ref, found := txs.engine.objects[key]
	if !found {
		return nil, false, nil
	}
	return &staleRetryObject{key: key, rootRef: ref}, true, nil
}

func (txs *staleRetryTx) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	panic("unexpected IterateObjects call")
}

func (txs *staleRetryTx) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
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

func (txs *staleRetryTx) Commit(ctx context.Context) error {
	if txs.engine.staleCommit > 0 {
		txs.engine.staleCommit--
		return coord.ErrStaleGeneration
	}
	maps.Copy(txs.engine.objects, txs.pending)
	return nil
}

func (txs *staleRetryTx) Discard() {}

type staleRetryObject struct {
	key     string
	rootRef *bucket.ObjectRef
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
