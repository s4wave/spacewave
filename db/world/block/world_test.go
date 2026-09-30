package world_block_test

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/block/filters"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestWorldEngine performs a simple test of operations against world engine.
func TestWorldEngine(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	eng, err := world_block.NewEngine(
		ctx,
		le,
		ocs,
		world_mock.LookupMockOp,
		nil,
		true,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// basic sanity tests
	err = world_mock.TestWorldEngine_Basic(ctx, le, eng)
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}

func TestWorldEngineCloseReleasesReadState(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	eng, err := world_block.NewEngine(
		ctx,
		le,
		ocs,
		world_mock.LookupMockOp,
		nil,
		false,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err.Error())
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := eng.NewTransaction(ctx, false); err == nil {
		t.Fatal("expected closed engine to reject read transaction")
	}
	if _, err := eng.BuildStorageCursor(ctx); err == nil {
		t.Fatal("expected closed engine to reject storage cursor")
	}
	if _, err := eng.GetSeqno(ctx); err == nil {
		t.Fatal("expected closed engine to reject seqno read")
	}
}

func TestObjectGetSubBlocksExposesRootRefForExtraction(t *testing.T) {
	rootRef := worldTestBlockRef(t, "world-object-root-ref")
	objRoot := &bucket.ObjectRef{RootRef: rootRef}
	obj := world_block.NewObject("object-with-root", objRoot)

	subBlocks := obj.GetSubBlocks()
	sub, ok := subBlocks[2]
	if !ok {
		t.Fatal("expected object root ref to be exposed as sub-block field 2")
	}
	gotRoot, ok := sub.(*bucket.ObjectRef)
	if !ok {
		t.Fatalf("object sub-block field 2 = %T, want *bucket.ObjectRef", sub)
	}
	if !gotRoot.GetRootRef().EqualsRef(rootRef) {
		t.Fatalf("object sub-block root = %s, want %s", gotRoot.GetRootRef().MarshalString(), rootRef.MarshalString())
	}

	refs, err := block.ExtractBlockRefs(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(refs) != 1 {
		t.Fatalf("ExtractBlockRefs returned %d refs, want 1", len(refs))
	}
	if !refs[0].EqualsRef(rootRef) {
		t.Fatalf("ExtractBlockRefs root = %s, want %s", refs[0].MarshalString(), rootRef.MarshalString())
	}
}

func worldTestBlockRef(t *testing.T, data string) *block.BlockRef {
	t.Helper()
	h, err := hash.Sum(hash.HashType_HashType_BLAKE3, []byte(data))
	if err != nil {
		t.Fatal(err.Error())
	}
	return block.NewBlockRef(h)
}

// TestWorldState_GetObjectMetadataBatch checks batched parent+type lookup behavior.
func TestWorldState_GetObjectMetadataBatch(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	oref := &bucket.ObjectRef{BucketId: "test-bucket"}
	for _, key := range []string{"parent", "child-a", "child-b", "child-c"} {
		{
			createdObject, err := ws.CreateObject(ctx, key, oref)
			world.ReleaseObjectState(createdObject)
			if err != nil {
				t.Fatal(err.Error())
			}
		}
	}

	if err := world_types.SetObjectType(ctx, ws, "child-a", "type/a"); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_types.SetObjectType(ctx, ws, "child-b", "type/b"); err != nil {
		t.Fatal(err.Error())
	}
	if err := world_parent.SetObjectParent(ctx, ws, "child-a", "parent", false); err != nil {
		t.Fatal(err.Error())
	}

	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	mds, err := world_types.GetObjectMetadataBatch(ctx, ws, []string{"child-b", "child-c", "child-a", "child-a"})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(mds) != 4 {
		t.Fatalf("expected 4 metadata results, got %d", len(mds))
	}

	checkMetadata := func(md *world_types.ObjectMetadata, key, typeID, parentKey string) {
		if md.ObjectKey != key || md.TypeID != typeID || md.ParentObjectKey != parentKey {
			t.Fatalf(
				"unexpected metadata for %s: got key=%q type=%q parent=%q",
				key,
				md.ObjectKey,
				md.TypeID,
				md.ParentObjectKey,
			)
		}
	}

	checkMetadata(mds[0], "child-b", "type/b", "")
	checkMetadata(mds[1], "child-c", "", "")
	checkMetadata(mds[2], "child-a", "type/a", "parent")
	checkMetadata(mds[3], "child-a", "type/a", "parent")
}

func TestWorldStateExplicitKVImplCompatibility(t *testing.T) {
	for _, impl := range []kvtx_block.KVImplType{
		kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL,
		kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA,
		kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE,
	} {
		t.Run(impl.String(), func(t *testing.T) {
			testWorldStateExplicitKVImplCompatibility(t, impl)
		})
	}
}

func TestWorldStateDefaultGraphKVTXUsesOkra(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ws.Discard()

	oref := &bucket.ObjectRef{BucketId: "test-bucket"}
	{
		createdObject, err := ws.CreateObject(ctx, "default/a", oref)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := ws.CreateObject(ctx, "default/b", oref)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	quad := world.NewGraphQuadWithKeys("default/a", "<default-rel>", "default/b", "")
	if err := ws.SetGraphQuad(ctx, quad); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	writtenRoot, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	assertWorldStoreImpl(t, "object", writtenRoot.GetObjectKeyValue(), kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL)
	assertWorldStoreImpl(t, "graph", writtenRoot.GetGraphKeyValue(), kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE)

	readWS, err := world_block.BuildMockWorldState(ctx, le, false, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readWS.Discard()
	quads, err := readWS.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("default/a", "<default-rel>", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 1 || quads[0].GetObj() != world.KeyToGraphValue("default/b").String() {
		t.Fatalf("LookupGraphQuads = %#v, want default/b", quads)
	}
}

func testWorldStateExplicitKVImplCompatibility(t *testing.T, impl kvtx_block.KVImplType) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	btx, bcs := ocs.BuildTransaction(nil)
	root := world_block.NewWorld(false)
	root.ObjectKeyValue = kvtx_block.NewKeyValueStore(impl)
	root.GraphKeyValue = kvtx_block.NewKeyValueStore(impl)
	bcs.SetBlock(root, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(rootRef)

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ws.Discard()

	oref := &bucket.ObjectRef{BucketId: "test-bucket"}
	{
		createdObject, err := ws.CreateObject(ctx, "explicit/a", oref)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := ws.CreateObject(ctx, "explicit/b", oref)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		objectState, err := world.MustGetObject(ctx, ws, "explicit/a")
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	quad := world.NewGraphQuadWithKeys("explicit/a", "<explicit-rel>", "explicit/b", "")
	retained, err := world.MustGetObject(ctx, ws, "explicit/a")
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(retained)
	beforeRoot, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := slices.Clone(beforeRoot.GetObjectKeyValue().GetOkraRoot().GetRootHash())
	if err := ws.SetGraphQuad(ctx, quad); err != nil {
		t.Fatal(err.Error())
	}
	if _, rev, err := retained.GetRootRef(ctx); err != nil || rev != 2 {
		t.Fatalf("retained object revision=%d err=%v", rev, err)
	}
	if _, err := retained.IncrementRev(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	writtenRoot, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	assertWorldKVImpl(t, writtenRoot, impl)
	if impl != kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL && bytes.Equal(beforeHash, writtenRoot.GetObjectKeyValue().GetOkraRoot().GetRootHash()) {
		t.Fatal("object mutation left the packed index hash unchanged")
	}

	readWS, err := world_block.BuildMockWorldState(ctx, le, false, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readWS.Discard()
	refs, err := readWS.GetObjectRootRefsBatch(ctx, []string{"explicit/a", "explicit/b"})
	if err != nil || refs[0].Rev != 3 || refs[1].Rev != 2 {
		t.Fatalf("object revisions after readback=%v err=%v", refs, err)
	}
	{
		objectState2, err := world.MustGetObject(ctx, readWS, "explicit/a")
		world.ReleaseObjectState(objectState2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	quads, err := readWS.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("explicit/a", "<explicit-rel>", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 1 || quads[0].GetObj() != world.KeyToGraphValue("explicit/b").String() {
		t.Fatalf("LookupGraphQuads = %#v, want explicit/b", quads)
	}
}

func assertWorldKVImpl(t *testing.T, root *world_block.World, impl kvtx_block.KVImplType) {
	t.Helper()
	stores := map[string]*kvtx_block.KeyValueStore{
		"object": root.GetObjectKeyValue(),
		"graph":  root.GetGraphKeyValue(),
	}
	for name, store := range stores {
		assertWorldStoreImpl(t, name, store, impl)
	}
}

func assertWorldStoreImpl(t *testing.T, name string, store *kvtx_block.KeyValueStore, impl kvtx_block.KVImplType) {
	t.Helper()
	if store == nil {
		t.Fatalf("%s store is nil", name)
	}
	if got := store.GetImplType(); got != impl {
		t.Fatalf("%s impl = %s, want %s", name, got, impl)
	}
}

// TestWorldState_GetObjectRootRefsBatch checks batched root-ref lookup behavior.
func TestWorldState_GetObjectRootRefsBatch(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	alphaRef := &bucket.ObjectRef{BucketId: "alpha-bucket"}
	betaRef := &bucket.ObjectRef{BucketId: "beta-bucket"}
	{
		createdObject, err := ws.CreateObject(ctx, "alpha", alphaRef)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := ws.CreateObject(ctx, "beta", betaRef)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	refs, err := world.GetObjectRootRefsBatch(ctx, ws, []string{"beta", "missing", "alpha", "alpha"})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(refs) != 4 {
		t.Fatalf("expected 4 root ref results, got %d", len(refs))
	}

	checkRootRef := func(ref *world.ObjectRootRef, key string, exists bool, bucketID string) {
		if ref.ObjectKey != key || ref.Exists != exists {
			t.Fatalf("unexpected root ref metadata for %s: got key=%q exists=%v", key, ref.ObjectKey, ref.Exists)
		}
		if !exists {
			if ref.RootRef != nil || ref.Rev != 0 {
				t.Fatalf("expected missing object %s to have empty root ref metadata", key)
			}
			return
		}
		if ref.RootRef.GetBucketId() != bucketID || ref.Rev != 1 {
			t.Fatalf(
				"unexpected root ref for %s: got bucket=%q rev=%d",
				key,
				ref.RootRef.GetBucketId(),
				ref.Rev,
			)
		}
	}

	checkRootRef(refs[0], "beta", true, "beta-bucket")
	checkRootRef(refs[1], "missing", false, "")
	checkRootRef(refs[2], "alpha", true, "alpha-bucket")
	checkRootRef(refs[3], "alpha", true, "alpha-bucket")
}

func TestWorldState_LookupGraphQuadsReturnsFullTypeQuad(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	{
		createdObject, err := ws.CreateObject(ctx, "repo-1", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := world_types.SetObjectType(ctx, ws, "repo-1", "git/repo"); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuad("<repo-1>", "<type>", "", ""),
		1,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 1 {
		t.Fatalf("expected 1 quad, got %d", len(quads))
	}
	if quads[0].GetObj() != "<types/git/repo>" {
		t.Fatalf("expected object %q, got %q", "<types/git/repo>", quads[0].GetObj())
	}
}

func TestWorldState_QueryGraphPathSeesUncommittedWrite(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ws.Discard()

	{
		createdObject, err := ws.CreateObject(ctx, "path-pending/a", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := ws.CreateObject(ctx, "path-pending/b", nil)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("path-pending/a", "<path-pending-rel>", "path-pending/b", "")); err != nil {
		t.Fatal(err.Error())
	}

	result, err := ws.QueryGraphPath(ctx, &world.GraphPathQuery{
		StartKeys: []string{"path-pending/a"},
		Steps: []world.GraphPathStep{
			{
				Direction: world.GraphPathDirectionOut,
				Predicate: "<path-pending-rel>",
				Limit:     10,
			},
		},
		ResultLimit:  10,
		IncludeQuads: true,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(result.ObjectKeys) != 1 || result.ObjectKeys[0] != "path-pending/b" {
		t.Fatalf("unexpected object keys: %#v", result.ObjectKeys)
	}
	if len(result.Quads) != 1 {
		t.Fatalf("unexpected quads: %#v", result.Quads)
	}
}

func TestWorldState_LookupGraphQuadsBatchSeesUncommittedWrite(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ws.Discard()

	{
		createdObject, err := ws.CreateObject(ctx, "batch-pending/a", nil)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := ws.CreateObject(ctx, "batch-pending/b", nil)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("batch-pending/a", "<batch-pending-rel>", "batch-pending/b", "")); err != nil {
		t.Fatal(err.Error())
	}

	results, err := ws.LookupGraphQuadsBatch(ctx, []world.GraphQuad{
		world.NewGraphQuadWithKeys("batch-pending/a", "<batch-pending-rel>", "", ""),
	}, 10)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(results) != 1 || len(results[0]) != 1 {
		t.Fatalf("unexpected batch results: %#v", results)
	}
	if results[0][0].GetObj() != "<batch-pending/b>" {
		t.Fatalf("unexpected object: %q", results[0][0].GetObj())
	}
}

// TestWorldState_DeleteObject tests the DeleteObject functionality
func TestWorldState_DeleteObject(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create two objects for graph deletion checks.
	objKey1 := "test-obj1"
	oref := &bucket.ObjectRef{BucketId: "test-bucket"}
	var createdObject world.ObjectState
	createdObject, err = ws.CreateObject(ctx, objKey1, oref)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey2 := "test-obj2"
	var createdObject2 world.ObjectState
	createdObject2, err = ws.CreateObject(ctx, objKey2, oref)
	world.ReleaseObjectState(createdObject2)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Add graph edges between the objects.
	err = ws.SetGraphQuad(ctx, world.NewGraphQuad(
		world.KeyToGraphValue(objKey1).String(),
		"<predicate1>",
		world.KeyToGraphValue(objKey2).String(),
		"",
	))
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.SetGraphQuad(ctx, world.NewGraphQuad(
		world.KeyToGraphValue(objKey2).String(),
		"<predicate2>",
		world.KeyToGraphValue(objKey1).String(),
		"",
	))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Commit the initial objects and graph edges.
	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Delete the first object and its graph edges.
	deleted, err := ws.DeleteObject(ctx, objKey1)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.FailNow()
	}

	// Commit the deletion before verifying persisted absence.
	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the deleted object is no longer addressable.
	var objectState world.ObjectState
	objectState, err = world.MustGetObject(ctx, ws, objKey1)
	world.ReleaseObjectState(objectState)
	if err == nil {
		t.Fatal("Expected error when getting deleted object, but got nil")
	}
	if !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("Expected ErrObjectNotFound, but got: %v", err)
	}

	// Verify graph edges involving the deleted object are gone.
	valueStr := world.KeyToGraphValue(objKey1).String()

	// Query quads where the deleted object was the subject.
	subjQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad(valueStr, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Query quads where the deleted object was the object.
	objQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad("", "", valueStr, ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(subjQuads) != 0 || len(objQuads) != 0 {
		t.Fatalf("expected DeleteGraphObject to delete quads for the object but got %d", len(subjQuads)+len(objQuads))
	}

	t.Log("DeleteObject test successful")
}

func TestWorldState_DisabledChangelogObjectOperations(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	btx, bcs := ocs.BuildTransaction(nil)
	bcs.ClearAllRefs()
	bcs.SetBlock(world_block.NewWorld(true), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(rootRef)

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "disabled-changelog-object"
	oref := &bucket.ObjectRef{BucketId: "test-bucket"}
	obj, err := ws.CreateObject(ctx, objKey, oref)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = obj.SetRootRef(ctx, &bucket.ObjectRef{BucketId: "test-bucket-next"})
	if err != nil {
		t.Fatal(err.Error())
	}
	deleted, err := ws.DeleteObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatalf("expected %q to be deleted", objKey)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	root, err := world_block.UnmarshalWorld(ctx, ws.GetBcs())
	if err != nil {
		t.Fatal(err.Error())
	}
	if !root.GetLastChangeDisable() {
		t.Fatal("expected changelog to remain disabled")
	}
	lastChange := root.GetLastChange().CloneVT()
	lastChange.Seqno = 0
	if lastChange.SizeVT() != 0 {
		t.Fatalf("expected disabled changelog to keep only seqno, got %s", lastChange.String())
	}
	if root.GetLastChange().GetSeqno() != 3 {
		t.Fatalf("last change seqno = %d, want 3", root.GetLastChange().GetSeqno())
	}
}

func TestWorldState_DeleteObjectRemovesLiteralPredicateQuads(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	rootRef := &bucket.ObjectRef{BucketId: "test-bucket"}
	imageKey := "image"
	oldAssetKey := "image-old-asset"
	{
		createdObject, err := ws.CreateObject(ctx, imageKey, rootRef)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := ws.CreateObject(ctx, oldAssetKey, rootRef)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(imageKey, "v86image/wasm", oldAssetKey, "")); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	deleted, err := ws.DeleteObject(ctx, imageKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatalf("expected %q to be deleted", imageKey)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(imageKey, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 0 {
		t.Fatalf("expected no outgoing quads for deleted image, got %d", len(quads))
	}
}

// TestWorldState_DeleteObjectWithMalformedGraphQuad verifies object deletion can clean up legacy graph quads.
func TestWorldState_DeleteObjectWithMalformedGraphQuad(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "delete-malformed-obj"
	otherKey := "delete-malformed-other"
	oref := &bucket.ObjectRef{BucketId: "test-bucket"}
	{
		createdObject, err := ws.CreateObject(ctx, objKey, oref)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	{
		createdObject2, err := ws.CreateObject(ctx, otherKey, oref)
		world.ReleaseObjectState(createdObject2)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	err = ws.AccessCayleyGraph(ctx, true, func(ctx context.Context, h world.CayleyHandle) error {
		w, ok := h.(graph.QuadWriter)
		if !ok {
			return errors.New("expected writable graph handle")
		}
		return w.AddQuad(ctx, quad.Quad{
			Subject: world.KeyToGraphValue(objKey),
			Object:  world.KeyToGraphValue(otherKey),
		})
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	deleted, err := ws.DeleteObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatalf("expected %q to be deleted", objKey)
	}
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(objKey, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 0 {
		t.Fatalf("expected malformed graph quad to be deleted, got %d", len(quads))
	}
}

func TestWorldState_ChangelogObjectSetStoresCurrentAndPreviousObjectRefs(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	obj, err := ws.CreateObject(ctx, "changelog-set-ref", &bucket.ObjectRef{BucketId: "initial-bucket"})
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := obj.SetRootRef(ctx, &bucket.ObjectRef{BucketId: "next-bucket"}); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	worldRoot, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	lastChange := worldRoot.GetLastChange()
	if ct := lastChange.GetChangeType(); ct != world_block.WorldChangeType_WorldChange_OBJECT_SET {
		t.Fatalf("expected last change type OBJECT_SET, got %s", ct.String())
	}

	var foundSetRootRefChange bool
	for _, change := range lastChange.GetChangeBatch().GetChanges() {
		if change.GetKey() != "changelog-set-ref" || change.GetPrevObjectRef().GetEmpty() {
			continue
		}
		foundSetRootRefChange = true
		if change.GetObjectRef().GetEmpty() {
			t.Fatal("expected SetRootRef changelog change to store object_ref")
		}
	}
	if !foundSetRootRefChange {
		t.Fatal("expected SetRootRef changelog change to store prev_object_ref")
	}
}

func TestWorldState_ChangelogDeleteObjectStoresPreviousObjectRef(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	{
		createdObject, err := ws.CreateObject(ctx, "changelog-delete-ref", &bucket.ObjectRef{BucketId: "deleted-bucket"})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	deleted, err := ws.DeleteObject(ctx, "changelog-delete-ref")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected object to be deleted")
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	worldRoot, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	lastChange := worldRoot.GetLastChange()
	if ct := lastChange.GetChangeType(); ct != world_block.WorldChangeType_WorldChange_OBJECT_DELETE {
		t.Fatalf("expected last change type OBJECT_DELETE, got %s", ct.String())
	}
	changes := lastChange.GetChangeBatch().GetChanges()
	if len(changes) != 1 {
		t.Fatalf("expected one object delete change, got %d", len(changes))
	}
	if changes[0].GetPrevObjectRef().GetEmpty() {
		t.Fatal("expected delete changelog change to store prev_object_ref")
	}
	if !changes[0].GetObjectRef().GetEmpty() {
		t.Fatal("expected delete changelog change not to store object_ref")
	}
}

// TestWorldEngine_Fork tests forking the block-backed world state.
//
// Applies the result to the original WorldState & checks.
func TestWorldEngine_Fork(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Seed the original state with a mock object.
	objKey := "tx-test-obj-1"
	var createdObject world.ObjectState
	createdObject, err = world_block.BuildMockObject(ctx, ws, objKey)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// Fork the state and apply a revision-changing operation.
	sender := tb.Volume.GetPeerID()
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err == nil {
		var objectState world.ObjectState
		objectState, err = world.MustGetObject(ctx, ws, objKey)
		world.ReleaseObjectState(objectState)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	forkedWs, err := ws.Fork(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	forked := forkedWs.(*world_block.WorldState)

	// Apply the operation to the fork.
	_, _, err = forked.ApplyWorldOp(
		ctx,
		world_mock.NewMockWorldOp(objKey, "hello there #2"),
		sender,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Define the revision assertion used for both states.
	checkRev := func(obj world.ObjectState, expected uint64) {
		if err := world.AssertObjectRev(ctx, obj, expected); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Verify the original state remains unchanged.
	obj, err := world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj)
	if err == nil {
		checkRev(obj, 1)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Commit the forked state.
	err = forked.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Publish the forked root and reopen the original state from it.
	ocs.SetRootRef(forked.GetRootRef())

	// A new block transaction forces a fresh cursor for the reopened state.
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the forked revision was published.
	obj, err = world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj)
	if err == nil {
		checkRev(obj, 2)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Report successful fork assertions.
	t.Log("tests successful")
}

// TestWorldEngine_UpdateRootRef tests updating the root ref while a write tx is active.
func TestWorldEngine_UpdateRootRef(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	eng, err := world_block.NewEngine(
		ctx,
		le,
		ocs,
		world_mock.LookupMockOp,
		nil,
		false,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "test-object"

	// Create and commit the initial object state.
	ws, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oref1 := &bucket.ObjectRef{BucketId: "test-1"}
	var createdObject world.ObjectState
	createdObject, err = ws.CreateObject(ctx, objKey, oref1)
	world.ReleaseObjectState(createdObject)
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Capture the first published root reference.
	state1 := eng.GetRootRef()

	// Increment the object revision and commit the updated state.
	ws, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	obj1, err := world.MustGetObject(ctx, ws, objKey)
	defer world.ReleaseObjectState(obj1)
	if err != nil {
		t.Fatal(err.Error())
	}
	rev2, err := obj1.IncrementRev(ctx)
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a read transaction for the updated revision.
	rtx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rtx.Discard()

	// Capture the second published root reference.
	state2 := eng.GetRootRef()

	// Verify the read transaction observes the committed revision.
	obj1, err = world.MustGetObject(ctx, rtx, objKey)
	defer world.ReleaseObjectState(obj1)
	if err == nil {
		var rev uint64
		_, rev, err = obj1.GetRootRef(ctx)
		if err == nil && rev != rev2 {
			err = errors.Errorf("expected rev %d but got %d", rev2, rev)
		}
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a write transaction that will be invalidated by root rollback.
	wtx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Roll back the engine root while retaining the reader's immutable revision.
	err = eng.SetRootRef(ctx, state1)

	// The existing reader remains at its captured revision.
	if err == nil {
		var rev uint64
		_, rev, err = obj1.GetRootRef(ctx)
		if err == nil && rev != rev2 {
			err = errors.Errorf("expected snapshot rev %d but got %d", rev2, rev)
		}
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	fresh, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Discard()
	rolledBack, err := world.MustGetObject(ctx, fresh, objKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(rolledBack)
	_, rollbackRev, err := rolledBack.GetRootRef(ctx)
	if err != nil || rollbackRev != rev2-1 {
		t.Fatalf("new reader after rollback: rev=%d err=%v", rollbackRev, err)
	}

	// Confirm the prior write transaction was discarded by the rollback.
	werr := wtx.Commit(ctx)
	if werr != tx.ErrDiscarded {
		t.Fatalf("expected discarded error, got %v", werr)
	}
	ws.Discard()

	// could check state2 again as well
	_ = state2

	// Report successful root update assertions.
	t.Log("tests successful")
}

// TestWorldState_Basic performs a simple test of operations against world.
func TestWorldState_Basic(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build a reusable example root reference for the object set.
	objRefCs := ocs.Clone()
	oref := objRefCs.GetRef()
	oref.BucketId = ""
	obtx, obcs := objRefCs.BuildTransaction(nil)
	obcs.SetBlock(block_mock.NewExampleBlock(), true)
	oref.RootRef, obcs, err = obtx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	nObjects := 100
	keys := make([]string, 0, nObjects)
	for i := range nObjects {
		keys = append(keys, "test-obj-"+strconv.Itoa(i))
	}
	forEachObj := func(cb func(objKey string) error) {
		for _, objKey := range keys {
			if err := cb(objKey); err != nil {
				t.Fatal(err.Error())
			}
		}
	}

	// Create all test objects in the world.
	forEachObj(func(objKey string) error {
		var createdObject world.ObjectState
		createdObject, err = ws.CreateObject(ctx, objKey, oref)
		world.ReleaseObjectState(createdObject)
		return err
	})

	// Read all test objects into iterator test state.
	var i int
	objStates := make([]world.ObjectState, len(keys))
	forEachObj(func(objKey string) error {
		var err error
		objStates[i], err = world.MustGetObject(ctx, ws, objKey)
		i++
		return err
	})

	// Update the shared root reference used by each object.
	obcs.SetBlock(&block_mock.SubBlock{ExamplePtr: oref.GetRootRef()}, true)
	oref.RootRef, obcs, err = obtx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = obcs

	// Publish the updated root reference on every object.
	for _, objState := range objStates {
		_, err = objState.SetRootRef(ctx, oref)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Increment every object's revision after updating its root.
	for _, objState := range objStates {
		_, err = objState.IncrementRev(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// add a graph quad
	err = ws.SetGraphQuad(ctx, world.NewGraphQuad(
		world.KeyToGraphValue(keys[0]).String(),
		"<mypredicate>",
		world.KeyToGraphValue(keys[4]).String(),
		"",
	))
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// success
	worldRoot, err := ws.GetRoot(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	lastChange := worldRoot.GetLastChange()
	lastChangeBcs := ws.GetBcs().FollowSubBlock(3)
	var changelogEntries []*world_block.ChangeLogLL
	for lastChange.GetSeqno() != 0 {
		changelogEntries = append(changelogEntries, lastChange)

		//  le.Infof("changelog entry: %s", lastChange.String())
		_ = lastChange

		lastChangeBcs = lastChangeBcs.FollowRef(2, lastChange.GetPrevRef())
		lastChange, err = world_block.UnmarshalChangeLogLL(ctx, lastChangeBcs)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Expect 3 changelog entries:
	// seqno=1: OBJECT_SET, prefix=test-obj-, key_bloom filter = <k:4, m:307, bit_set...
	// seqno=2: OBJECT_INC_REV: prefix=test-obj-, key_bloom = same as first.
	// seqno=3: OBJECT_GRAPH_SET
	if len(changelogEntries) != 3 {
		t.Fatalf("expected 3 changelog entries but found %d", len(changelogEntries))
	}
	for i, ent := range changelogEntries {
		if kp := ent.GetKeyFilters().GetKeyPrefix(); kp != "test-obj-" && i != 0 {
			t.Fatalf("%d: key prefix expected test-obj- but got %s", i, kp)
		}
		keyBloomReader := filters.NewKeyFiltersReader(ent.GetKeyFilters())
		forEachObj(func(objKey string) error {
			if !keyBloomReader.TestObjectKey(objKey) {
				return errors.Errorf("expected bloom to contain %q but did not", objKey)
			}
			return nil
		})

		if int(ent.GetSeqno()) != 3-i { //nolint:gosec
			t.Fatalf("%d: seqno expected %d but got %d", i, 3-i, ent.GetSeqno())
		}
		chn := len(ent.GetChangeBatch().GetChanges())
		if chn > world_block.HeadChangeCountLimit {
			t.Fatalf("%d: changes in-line expected max %d but got %d", i, world_block.HeadChangeCountLimit, chn)
		} else {
			t.Logf("%d: %d changes were in the HEAD block", i, chn)
		}
		if i != 0 {
			if ent.GetChangeBatch().GetPrevRef().GetEmpty() {
				t.Logf("%d: expected prev_ref on change batch but was empty", i)
			}
			if ts := int(ent.GetChangeBatch().GetTotalSize()); ts != nObjects {
				t.Fatalf("%d: total size expected %d but got %d", i, nObjects, ts)
			}
		}
	}
	if !changelogEntries[len(changelogEntries)-1].GetPrevRef().GetEmpty() {
		t.Fatal("expected prev_ref empty on first change")
	}
	if changelogEntries[0].GetPrevRef().GetEmpty() {
		t.Fatal("expected prev_ref on last change")
	}
}

// commitObjectInEngine creates one object through an engine block transaction
// and advances the engine root, failing the test on any error.
func commitObjectInEngine(t *testing.T, ctx context.Context, eng *world_block.Engine, key string) {
	t.Helper()
	btx, err := eng.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer btx.Discard()
	{
		createdObject, err := btx.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "test"})
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	ref, err := btx.CommitBlockTransaction(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := eng.SetRootRef(ctx, ref); err != nil {
		t.Fatal(err.Error())
	}
}

// TestEngineDeferredDurabilityCrashRecovery proves the single-writer deferred
// durability fence: a per-commit write advances only the in-memory root, Sync
// runs the block barrier and then advances the durable head, and a crash (engine
// drop) before the next Sync rolls recovery back to the last Sync'd head with all
// of its blocks present and the unsynced commit gone.
func TestEngineDeferredDurabilityCrashRecovery(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	cur, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// commitFn stands in for the durable head write (writeHeadState). It records
	// the head and the call count so the test can assert the per-commit cadence.
	var durableHead *bucket.ObjectRef
	var commitCount int
	commitFn := func(_ context.Context, _, nref *bucket.ObjectRef) error {
		commitCount++
		durableHead = nref.Clone()
		return nil
	}

	eng, err := world_block.NewEngine(ctx, le, cur, world_mock.LookupMockOp, commitFn, false, world_block.WithDeferredDurability())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Tick 1: a deferred commit advances only the in-memory root.
	commitObjectInEngine(t, ctx, eng, "obj-a")
	if commitCount != 0 {
		t.Fatalf("deferred commit must not advance the durable head, got %d head writes", commitCount)
	}
	rootAfterA := eng.GetRootRef().Clone()

	// Sync fences the block barrier then advances the durable head to obj-a.
	if _, err := eng.Sync(ctx); err != nil {
		t.Fatal(err.Error())
	}
	if commitCount != 1 {
		t.Fatalf("Sync must advance the durable head exactly once, got %d", commitCount)
	}
	if durableHead == nil || !durableHead.EqualsRef(rootAfterA) {
		t.Fatal("durable head must equal the in-memory root after Sync")
	}
	syncedHead := durableHead.Clone()

	// Tick 2: another deferred commit; the durable head must still lag at obj-a.
	commitObjectInEngine(t, ctx, eng, "obj-b")
	if commitCount != 1 {
		t.Fatalf("post-Sync deferred commit must not advance the durable head, got %d", commitCount)
	}

	// Crash: drop the engine without a fence. The obj-b blocks live only in the
	// in-memory buffer and are lost; the durable head still names obj-a.
	if err := eng.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Recover: reopen a world state at the durable head over the same volume.
	cur2, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cur2.Release()
	cur2.SetRootRef(syncedHead.GetRootRef())
	recovered, err := world_block.BuildMockWorldState(ctx, le, false, cur2, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer recovered.Discard()

	// obj-a's blocks were fenced durable by Sync, so it recovers (this read also
	// proves block-before-head ordering: the head names only durable blocks)...
	{
		objectState, err := world.MustGetObject(ctx, recovered, "obj-a")
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatalf("recovery must land on the last Sync'd head with its blocks present: %v", err.Error())
		}
	}

	// ...and obj-b, committed after the last Sync, is rolled back.
	{
		objectState2, err := world.MustGetObject(ctx, recovered, "obj-b")
		world.ReleaseObjectState(objectState2)
		if err == nil {
			t.Fatal("post-Sync commit must not survive a crash before the next Sync")
		}
	}
}

func TestEngineTxObjectBodyPagePairsSeqnoWithBodies(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	const objectKey = "body/race"
	initialBody := []byte("version-initial")
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, tx, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(byteslice.NewByteSlice(&initialBody), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	initialSeqno, err := tx.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	versions := map[uint64]string{initialSeqno: string(initialBody)}

	pager, ok := tx.(world.ObjectBodyPageSeqnoBatcher)
	if !ok {
		t.Fatal("transaction does not expose body page seqno batching")
	}

	const (
		keyCount = 2048
		rounds   = 64
	)
	keys := make([]string, keyCount)
	for i := range keys {
		keys[i] = objectKey
	}

	type pageResult struct {
		seqno  uint64
		bodies []string
	}
	start := make(chan struct{})
	results := make(chan pageResult, rounds)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start
		for range rounds {
			bodies, _, seqno, err := pager.GetObjectBodiesBatchPageWithSeqno(
				ctx,
				keys,
				world.ObjectBodiesBatchByteBudget,
			)
			if err != nil {
				errs <- err
				return
			}
			if len(bodies) != len(keys) {
				errs <- errors.Errorf("body count = %d, want %d", len(bodies), len(keys))
				return
			}
			page := pageResult{seqno: seqno, bodies: make([]string, len(bodies))}
			for i, body := range bodies {
				page.bodies[i] = string(body.Body)
			}
			results <- page
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for range rounds {
			before, err := tx.GetSeqno(ctx)
			if err != nil {
				errs <- err
				return
			}
			body := []byte("version-" + strconv.FormatUint(before+1, 10))
			_, _, err = world.AccessWorldObject(ctx, tx, objectKey, true, func(bcs *block.Cursor) error {
				bcs.SetBlock(byteslice.NewByteSlice(&body), true)
				return nil
			})
			if err != nil {
				errs <- err
				return
			}
			after, err := tx.GetSeqno(ctx)
			if err != nil {
				errs <- err
				return
			}
			versions[after] = string(body)
		}
	}()

	close(start)
	wg.Wait()
	close(results)
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}

	for page := range results {
		expected, ok := versions[page.seqno]
		if !ok {
			t.Fatalf("page returned unknown seqno %d", page.seqno)
		}
		for i, body := range page.bodies {
			if body != expected {
				t.Fatalf(
					"body at index %d paired with seqno %d = %q, want %q",
					i,
					page.seqno,
					body,
					expected,
				)
			}
		}
	}
}

// TestEngineDefaultDurableOnWrite proves the default single-writer engine (no
// deferred-durability opt-in) keeps durable-on-write semantics: each commit
// advances the durable head immediately. This is the contract the SharedObject,
// CDN, and CLI engines depend on for cross-participant block availability.
func TestEngineDefaultDurableOnWrite(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	cur, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	var commitCount int
	commitFn := func(_ context.Context, _, _ *bucket.ObjectRef) error {
		commitCount++
		return nil
	}

	eng, err := world_block.NewEngine(ctx, le, cur, world_mock.LookupMockOp, commitFn, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer eng.Close()

	commitObjectInEngine(t, ctx, eng, "obj-a")
	if commitCount != 1 {
		t.Fatalf("default single-writer must advance the durable head per commit, got %d", commitCount)
	}
	commitObjectInEngine(t, ctx, eng, "obj-b")
	if commitCount != 2 {
		t.Fatalf("default single-writer must advance the durable head per commit, got %d", commitCount)
	}
}
