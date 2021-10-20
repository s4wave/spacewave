package world_block

import (
	"context"
	"strconv"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/filters"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

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

	ws, err := BuildMockWorldState(ctx, le, true, ocs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// construct a basic example object
	objRefCs := ocs.Clone()
	oref := objRefCs.GetRef()
	oref.BucketId = ""
	obtx, obcs := objRefCs.BuildTransaction(nil)
	obcs.SetBlock(block_mock.NewExampleBlock(), true)
	oref.RootRef, obcs, err = obtx.Write(true)
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
		_, err = ws.CreateObject(objKey, oref)
		return err
	})

	// lookup the objects
	var i int
	objStates := make([]world.ObjectState, len(keys))
	forEachObj(func(objKey string) error {
		var err error
		objStates[i], err = world.MustGetObject(ws, objKey)
		i++
		return err
	})

	// adjust object ref
	obcs.SetBlock(&block_mock.SubBlock{ExamplePtr: oref.GetRootRef()}, true)
	oref.RootRef, obcs, err = obtx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// adjust ref in the state
	for _, objState := range objStates {
		_, err = objState.SetRootRef(oref)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// increment rev
	for _, objState := range objStates {
		_, err = objState.IncrementRev()
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// add a graph quad
	err = ws.SetGraphQuad(world.NewGraphQuad(
		world.KeyToGraphValue(keys[0]).String(),
		"<mypredicate>",
		world.KeyToGraphValue(keys[4]).String(),
		"",
	))
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.Commit()
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// success
	worldRoot, err := ws.getRoot()
	if err != nil {
		t.Fatal(err.Error())
	}
	lastChange := worldRoot.GetLastChange()
	lastChangeBcs := ws.bcs.FollowSubBlock(3)
	var changelogEntries []*ChangeLogLL
	var changelogEntriesBcs []*block.Cursor
	for lastChange.GetSeqno() != 0 {
		changelogEntries = append(changelogEntries, lastChange)
		changelogEntriesBcs = append(changelogEntriesBcs, lastChangeBcs)

		le.Infof("changelog entry: %s", lastChange.String())
		lastChangeBcs = lastChangeBcs.FollowRef(2, lastChange.GetPrevRef())
		lastChange, err = UnmarshalChangeLogLL(lastChangeBcs)
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
		if chn > headChangeCountLimit {
			t.Fatalf("%d: changes in-line expected max %d but got %d", i, headChangeCountLimit, chn)
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
