package world_block_test

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/filters"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_types "github.com/s4wave/spacewave/db/world/types"
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
		if _, err := ws.CreateObject(ctx, key, oref); err != nil {
			t.Fatal(err.Error())
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

	// Create a test object
	objKey1 := "test-obj1"
	oref := &bucket.ObjectRef{BucketId: "test-bucket"}
	_, err = ws.CreateObject(ctx, objKey1, oref)
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey2 := "test-obj2"
	_, err = ws.CreateObject(ctx, objKey2, oref)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Add some graph quads related to the object
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

	// Commit the changes
	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Delete the object
	deleted, err := ws.DeleteObject(ctx, objKey1)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.FailNow()
	}

	// Commit the deletion
	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that the object no longer exists
	_, err = world.MustGetObject(ctx, ws, objKey1)
	if err == nil {
		t.Fatal("Expected error when getting deleted object, but got nil")
	}
	if !errors.Is(err, world.ErrObjectNotFound) {
		t.Fatalf("Expected ErrObjectNotFound, but got: %v", err)
	}

	// Verify that the quads related to the object are deleted
	valueStr := world.KeyToGraphValue(objKey1).String()

	// find all matching quads where subject == value
	subjQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad(valueStr, "", "", ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}

	// find all matching quads where object == value
	objQuads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuad("", "", valueStr, ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}

	if len(subjQuads) != 0 || len(objQuads) != 0 {
		t.Fatalf("expected DeleteGraphObject to delete quads for the object but got %d", len(subjQuads)+len(objQuads))
	}

	t.Log("DeleteObject test successful")
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

	// add the mock object
	objKey := "tx-test-obj-1"
	_, err = world_block.BuildMockObject(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// test forking it + applying changes
	sender := tb.Volume.GetPeerID()
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err == nil {
		_, err = world.MustGetObject(ctx, ws, objKey)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	forkedWs, err := ws.Fork(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	forked := forkedWs.(*world_block.WorldState)

	// apply operation, after, rev=3
	_, _, err = forked.ApplyWorldOp(
		ctx,
		world_mock.NewMockWorldOp(objKey, "hello there #2"),
		sender,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// checkRev asserts a object is at a revision
	checkRev := func(obj world.ObjectState, expected uint64) {
		if err := world.AssertObjectRev(ctx, obj, expected); err != nil {
			t.Fatal(err.Error())
		}
	}

	// ensure original state was still at rev=1
	obj, err := world.MustGetObject(ctx, ws, objKey)
	if err == nil {
		checkRev(obj, 1)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// write the forked state
	err = forked.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// apply the updated ref to the original state.
	ocs.SetRootRef(forked.GetRootRef())
	// note: we need a new block transaction to force a new cursor
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check if it was applied
	obj, err = world.MustGetObject(ctx, ws, objKey)
	if err == nil {
		checkRev(obj, 2)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
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

	// create the object in the world
	ws, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oref1 := &bucket.ObjectRef{BucketId: "test-1"}
	_, err = ws.CreateObject(ctx, objKey, oref1)
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// save the first state
	state1 := eng.GetRootRef()

	// change the object
	ws, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	obj1, err := world.MustGetObject(ctx, ws, objKey)
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

	// create a new read tx
	rtx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// save the second state
	state2 := eng.GetRootRef()

	// ensure the rev is correct
	obj1, err = world.MustGetObject(ctx, rtx, objKey)
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

	// create a write tx
	wtx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// change back to the original state
	err = eng.SetRootRef(ctx, state1)
	// use the same read tx to get the current rev
	if err == nil {
		var rev uint64
		_, rev, err = obj1.GetRootRef(ctx)
		if err == nil && rev != rev2-1 {
			err = errors.Errorf("expected rev %d - 1 = %d but got %d", rev2, rev2-1, rev)
		}
	}
	if err != nil {
		t.Fatal(err.Error())
	}
	// expect the write tx to have been discarded
	werr := wtx.Commit(ctx)
	if werr != tx.ErrDiscarded {
		t.Fatalf("expected discarded error, got %v", werr)
	}
	ws.Discard()

	// could check state2 again as well
	_ = state2

	// success
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

	// construct a basic example object
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

	// create the objects in the world
	forEachObj(func(objKey string) error {
		_, err = ws.CreateObject(ctx, objKey, oref)
		return err
	})

	// lookup the objects
	var i int
	objStates := make([]world.ObjectState, len(keys))
	forEachObj(func(objKey string) error {
		var err error
		objStates[i], err = world.MustGetObject(ctx, ws, objKey)
		i++
		return err
	})

	// adjust object ref
	obcs.SetBlock(&block_mock.SubBlock{ExamplePtr: oref.GetRootRef()}, true)
	oref.RootRef, obcs, err = obtx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = obcs

	// adjust ref in the state
	for _, objState := range objStates {
		_, err = objState.SetRootRef(ctx, oref)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// increment rev
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

// buildGCTestWorld creates a writable WorldState for GC testing.
func buildGCTestWorld(t *testing.T) (*world_block.WorldState, *testbed.Testbed) {
	t.Helper()
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
	t.Cleanup(ocs.Release)

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ws, tb
}

// TestWorldState_GC_RefGraphInit verifies that a writable WorldState
// initializes the RefGraph with a gcroot -> world edge.
func TestWorldState_GC_RefGraphInit(t *testing.T) {
	ctx := context.Background()
	ws, _ := buildGCTestWorld(t)

	rg := ws.GetRefGraph()
	if rg == nil {
		t.Fatal("expected RefGraph to be initialized for writable WorldState")
	}

	outgoing, err := rg.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Contains(outgoing, "world") {
		t.Fatalf("expected gcroot -> world edge, got outgoing: %v", outgoing)
	}
}

// TestWorldState_GC_CreateObject verifies that CreateObject adds
// world -> object and object -> block gc/ref edges.
func TestWorldState_GC_CreateObject(t *testing.T) {
	ctx := context.Background()
	ws, _ := buildGCTestWorld(t)
	rg := ws.GetRefGraph()
	if rg == nil {
		t.Fatal("no refgraph")
	}

	_, err := world_block.BuildMockObject(ctx, ws, "gc-test-obj")
	if err != nil {
		t.Fatal(err.Error())
	}

	objIRI := block_gc.ObjectIRI("gc-test-obj")

	// world -> object edge should exist
	outgoing, err := rg.GetOutgoingRefs(ctx, "world")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Contains(outgoing, objIRI) {
		t.Fatalf("expected world -> %s edge, got: %v", objIRI, outgoing)
	}

	// object -> block edge should exist
	objOutgoing, err := rg.GetOutgoingRefs(ctx, objIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(objOutgoing) == 0 {
		t.Fatal("expected object -> block gc/ref edge after CreateObject")
	}
	for _, ref := range objOutgoing {
		if len(ref) < len("block:") {
			t.Fatalf("expected block IRI, got: %s", ref)
		}
	}
}

// TestWorldState_GC_DeleteObject verifies that DeleteObject marks the
// object unreferenced and GarbageCollect sweeps it.
func TestWorldState_GC_DeleteObject(t *testing.T) {
	ctx := context.Background()
	ws, _ := buildGCTestWorld(t)
	rg := ws.GetRefGraph()
	if rg == nil {
		t.Fatal("no refgraph")
	}

	_, err := world_block.BuildMockObject(ctx, ws, "gc-del-obj")
	if err != nil {
		t.Fatal(err.Error())
	}

	objIRI := block_gc.ObjectIRI("gc-del-obj")

	// Verify object is referenced before delete.
	has, err := rg.HasIncomingRefs(ctx, objIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !has {
		t.Fatal("object should have incoming refs before delete")
	}

	// Delete the object.
	deleted, err := ws.DeleteObject(ctx, "gc-del-obj")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected object to be deleted")
	}

	// Object should now be unreferenced.
	unrefs, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Contains(unrefs, objIRI) {
		t.Fatalf("expected %s in unreferenced nodes, got: %v", objIRI, unrefs)
	}

	// GarbageCollect should sweep the object and its blocks.
	stats, err := ws.GarbageCollect(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept == 0 {
		t.Fatal("expected GarbageCollect to sweep at least the object node")
	}

	// After GC, no unreferenced nodes should remain.
	unrefs, err = rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(unrefs) != 0 {
		t.Fatalf("expected 0 unreferenced after GC, got: %v", unrefs)
	}
}

// TestWorldState_GC_SetRootRef verifies that SetRootRef swaps the
// object -> block gc/ref edge from old to new root.
func TestWorldState_GC_SetRootRef(t *testing.T) {
	ctx := context.Background()
	ws, _ := buildGCTestWorld(t)
	rg := ws.GetRefGraph()
	if rg == nil {
		t.Fatal("no refgraph")
	}

	objState, err := world_block.BuildMockObject(ctx, ws, "gc-swap-obj")
	if err != nil {
		t.Fatal(err.Error())
	}

	objIRI := block_gc.ObjectIRI("gc-swap-obj")

	// Record initial object -> block edges.
	oldOutgoing, err := rg.GetOutgoingRefs(ctx, objIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(oldOutgoing) == 0 {
		t.Fatal("expected object -> block edge after create")
	}

	// Build a new root block and set it.
	err = ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		oref := bls.GetRef()
		obtx, obcs := bls.BuildTransactionAtRef(nil, nil)
		obcs.SetBlock(&block_mock.Example{Msg: "updated root"}, true)
		var err error
		oref.RootRef, _, err = obtx.Write(ctx, true)
		if err != nil {
			return err
		}
		_, err = objState.SetRootRef(ctx, oref)
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// New outgoing should differ from old.
	newOutgoing, err := rg.GetOutgoingRefs(ctx, objIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(newOutgoing) == 0 {
		t.Fatal("expected object -> block edge after SetRootRef")
	}

	// Check that the old block IRI was replaced.
	oldSet := make(map[string]bool, len(oldOutgoing))
	for _, o := range oldOutgoing {
		oldSet[o] = true
	}
	newSet := make(map[string]bool, len(newOutgoing))
	for _, n := range newOutgoing {
		newSet[n] = true
	}
	if len(oldSet) == len(newSet) {
		same := true
		for k := range oldSet {
			if !newSet[k] {
				same = false
				break
			}
		}
		if same {
			t.Fatal("expected object -> block edges to change after SetRootRef")
		}
	}
}

// TestWorldState_GC_SetRootRef_OrphanBlock verifies that SetRootRef marks
// the old block unreferenced when no other object references it, and that
// GarbageCollect sweeps the orphaned block.
func TestWorldState_GC_SetRootRef_OrphanBlock(t *testing.T) {
	ctx := context.Background()
	ws, _ := buildGCTestWorld(t)
	rg := ws.GetRefGraph()
	if rg == nil {
		t.Fatal("no refgraph")
	}

	objState, err := world_block.BuildMockObject(ctx, ws, "gc-orphan-obj")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Get the old block IRI before SetRootRef.
	oref, _, err := objState.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	oldBlockIRI := block_gc.BlockIRI(oref.GetRootRef())
	if oldBlockIRI == "" {
		t.Fatal("expected non-empty old block IRI")
	}

	// Old block should NOT be unreferenced (it's referenced by the object).
	unrefs, err := rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if slices.Contains(unrefs, oldBlockIRI) {
		t.Fatal("old block should not be unreferenced before SetRootRef")
	}

	// SetRootRef to a new block.
	err = ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		nref := bls.GetRef()
		obtx, obcs := bls.BuildTransactionAtRef(nil, nil)
		obcs.SetBlock(&block_mock.Example{Msg: "new root block"}, true)
		var err error
		nref.RootRef, _, err = obtx.Write(ctx, true)
		if err != nil {
			return err
		}
		_, err = objState.SetRootRef(ctx, nref)
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Old block should now be unreferenced.
	unrefs, err = rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Contains(unrefs, oldBlockIRI) {
		t.Fatalf("expected old block %s in unreferenced nodes after SetRootRef, got: %v", oldBlockIRI, unrefs)
	}

	// GarbageCollect should sweep the orphaned old block.
	stats, err := ws.GarbageCollect(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept == 0 {
		t.Fatal("expected GC to sweep the orphaned old block")
	}

	// Old block should no longer be unreferenced (it was swept).
	unrefs, err = rg.GetUnreferencedNodes(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if slices.Contains(unrefs, oldBlockIRI) {
		t.Fatal("old block should have been swept by GC")
	}
}

// TestWorldState_GC_Fork verifies that forking a WorldState preserves
// GC tracking: the forked state has a RefGraph, existing GC edges are
// visible, new objects get GC edges, and GarbageCollect works.
func TestWorldState_GC_Fork(t *testing.T) {
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
	t.Cleanup(ocs.Release)

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create an object before fork.
	_, err = world_block.BuildMockObject(ctx, ws, "pre-fork-obj")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Commit so block data is persisted for fork.
	err = ws.Commit(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// Reload state to pick up committed data.
	ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Fork.
	forkedWs, err := ws.Fork(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	forked := forkedWs.(*world_block.WorldState)

	// Forked state should have a RefGraph.
	rg := forked.GetRefGraph()
	if rg == nil {
		t.Fatal("forked WorldState should have a RefGraph")
	}

	// gcroot -> world edge should exist in forked state.
	outgoing, err := rg.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Contains(outgoing, "world") {
		t.Fatalf("forked state: expected gcroot -> world edge, got: %v", outgoing)
	}

	// Pre-fork object should have world -> object edge.
	preForkIRI := block_gc.ObjectIRI("pre-fork-obj")
	worldOut, err := rg.GetOutgoingRefs(ctx, "world")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Contains(worldOut, preForkIRI) {
		t.Fatalf("forked state: expected world -> %s edge, got: %v", preForkIRI, worldOut)
	}

	// Pre-fork object should have object -> block edges.
	objOut, err := rg.GetOutgoingRefs(ctx, preForkIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(objOut) == 0 {
		t.Fatal("forked state: pre-fork object should have block refs")
	}

	// Create a new object in the forked state.
	_, err = world_block.BuildMockObject(ctx, forked, "post-fork-obj")
	if err != nil {
		t.Fatal(err.Error())
	}

	// New object should have GC edges in forked state.
	postForkIRI := block_gc.ObjectIRI("post-fork-obj")
	has, err := rg.HasIncomingRefs(ctx, postForkIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !has {
		t.Fatal("forked state: post-fork object should have incoming refs")
	}

	// Delete post-fork object and GC in forked state.
	deleted, err := forked.DeleteObject(ctx, "post-fork-obj")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected delete to succeed in forked state")
	}
	stats, err := forked.GarbageCollect(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept == 0 {
		t.Fatal("forked state: expected GC to sweep deleted object")
	}

	// Pre-fork object should still be intact after GC.
	has, err = rg.HasIncomingRefs(ctx, preForkIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !has {
		t.Fatal("forked state: pre-fork object should survive GC of post-fork object")
	}
}

// TestWorldState_GC_FullLifecycle tests create, update, delete, collect
// end-to-end: two objects, delete one, GC, verify the other survives.
func TestWorldState_GC_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	ws, _ := buildGCTestWorld(t)
	rg := ws.GetRefGraph()
	if rg == nil {
		t.Fatal("no refgraph")
	}

	// Create two objects.
	_, err := world_block.BuildMockObject(ctx, ws, "obj-keep")
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = world_block.BuildMockObject(ctx, ws, "obj-delete")
	if err != nil {
		t.Fatal(err.Error())
	}

	keepIRI := block_gc.ObjectIRI("obj-keep")
	delIRI := block_gc.ObjectIRI("obj-delete")

	// Both should be referenced by world.
	for _, iri := range []string{keepIRI, delIRI} {
		has, err := rg.HasIncomingRefs(ctx, iri)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !has {
			t.Fatalf("expected %s to have incoming refs", iri)
		}
	}

	// Delete one.
	deleted, err := ws.DeleteObject(ctx, "obj-delete")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected delete to succeed")
	}

	// GC sweeps the deleted object and its blocks.
	stats, err := ws.GarbageCollect(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.NodesSwept == 0 {
		t.Fatal("expected GC to sweep deleted object")
	}

	// Kept object should still be referenced.
	has, err := rg.HasIncomingRefs(ctx, keepIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !has {
		t.Fatal("kept object lost its gc/ref edges after GC")
	}

	// Kept object should still have outgoing block refs.
	keepOut, err := rg.GetOutgoingRefs(ctx, keepIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(keepOut) == 0 {
		t.Fatal("kept object should still have block refs")
	}

	// Deleted object should have no refs at all.
	delOut, err := rg.GetOutgoingRefs(ctx, delIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(delOut) != 0 {
		t.Fatalf("deleted object should have no outgoing refs, got: %v", delOut)
	}
	delIn, err := rg.GetIncomingRefs(ctx, delIRI)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(delIn) != 0 {
		t.Fatalf("deleted object should have no incoming refs, got: %v", delIn)
	}
}

// TestWorldState_GC_SweepTx verifies that a TxGCSweep transaction executed
// through the EngineTx path sweeps unreferenced objects.
func TestWorldState_GC_SweepTx(t *testing.T) {
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

	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	sender := tb.Volume.GetPeerID()
	objKey := "sweep-tx-obj"

	// Create an object via engine tx.
	{
		btx, err := eng.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()
		_, err = btx.CreateObject(ctx, objKey, &bucket.ObjectRef{BucketId: "test"})
		if err != nil {
			t.Fatal(err.Error())
		}
		ref, err := btx.CommitBlockTransaction(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := eng.SetRootRef(ctx, ref); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Delete the object via engine tx.
	{
		btx, err := eng.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()
		deleted, err := btx.DeleteObject(ctx, objKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !deleted {
			t.Fatal("expected object to be deleted")
		}
		ref, err := btx.CommitBlockTransaction(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := eng.SetRootRef(ctx, ref); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Verify unreferenced nodes exist before sweep.
	{
		ocs.SetRootRef(eng.GetRootRef().GetRootRef())
		ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		rg := ws.GetRefGraph()
		if rg == nil {
			t.Fatal("no refgraph on world state")
		}
		unrefs, err := rg.GetUnreferencedNodes(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		objIRI := block_gc.ObjectIRI(objKey)
		if !slices.Contains(unrefs, objIRI) {
			t.Fatalf("expected %s in unreferenced nodes before sweep, got: %v", objIRI, unrefs)
		}
	}

	// Execute GC sweep tx through the engine tx path.
	{
		sweepTx, err := world_block_tx.NewTxGCSweep()
		if err != nil {
			t.Fatal(err.Error())
		}
		ttx, err := sweepTx.LocateTx()
		if err != nil {
			t.Fatal(err.Error())
		}
		btx, err := eng.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer btx.Discard()
		_, err = ttx.ExecuteTx(ctx, sender, world_mock.LookupMockOp, btx)
		if err != nil {
			t.Fatal(err.Error())
		}
		ref, err := btx.CommitBlockTransaction(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := eng.SetRootRef(ctx, ref); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Verify no unreferenced nodes remain after sweep.
	{
		ocs.SetRootRef(eng.GetRootRef().GetRootRef())
		ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		rg := ws.GetRefGraph()
		if rg == nil {
			t.Fatal("no refgraph on world state")
		}
		unrefs, err := rg.GetUnreferencedNodes(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(unrefs) != 0 {
			t.Fatalf("expected no unreferenced nodes after sweep, got: %v", unrefs)
		}
	}
}
