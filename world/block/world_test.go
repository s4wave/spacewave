package world_block_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/filters"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
	"github.com/pkg/errors"
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

// TestNewAccessWatchableObjectState tests the NewAccessWatchableObjectState function
func TestNewAccessWatchableObjectState(t *testing.T) {
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
	objKey := "test-watchable-obj"
	obj, err := world_block.BuildMockObject(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the access function
	unmarshal := func(ctx context.Context, bcs *block.Cursor) (*world_block.MockObject, error) {
		return world_block.UnmarshalMockObject(ctx, bcs)
	}
	accessFunc := world.NewAccessWatchableObjectState(obj, unmarshal)

	// Use the access function
	watchable, release, err := accessFunc(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer release()

	// Get the initial state
	initialState := watchable.GetValue()
	if initialState.GetMsg() == "" {
		t.Fatal("Unexpected empty initial state")
	}

	// Update the object state
	nextMsg := "updated value"
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(&world_block.MockObject{Msg: nextMsg}, true)
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check the updated state
	updatedState, err := watchable.WaitValueChange(ctx, initialState, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if updMsg := updatedState.GetMsg(); updMsg != nextMsg {
		t.Fatalf("Expected updated state %s, got: %s", nextMsg, updMsg)
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
	for i := 0; i < nObjects; i++ {
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
		if int(ent.GetSeqno()) != 3-i {
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
