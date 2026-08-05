package world_mock

import (
	"bytes"
	"context"
	"strconv"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/path"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// TestWorldEngine applies all tests to the world engine.
func TestWorldEngine(ctx context.Context, le *logrus.Entry, eng world.Engine) error {
	tests := [](func(ctx context.Context, le *logrus.Entry, eng world.Engine) error){
		TestWorldEngine_Basic,
	}
	for _, t := range tests {
		err := t(ctx, le, eng)
		if err != nil {
			return err
		}
	}
	return nil
}

// TestWorldEngine_Basic performs basic sanity tests on a world engine.
func TestWorldEngine_Basic(ctx context.Context, le *logrus.Entry, eng world.Engine) error {
	objKey := "test-object"

	// Create the initial object in a writable transaction.
	ws, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	oref1 := &bucket.ObjectRef{BucketId: "test-1"}
	_, err = ws.CreateObject(ctx, objKey, oref1)
	if err != nil {
		return errors.Wrapf(err, "create object: %s", objKey)
	}

	// Read back the created object and verify its root reference.
	objState, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return errors.Wrapf(err, "get object: %s", objKey)
	}

	assertEqual := func(o1, o2 *bucket.ObjectRef) error {
		if o1.GetBucketId() != o2.GetBucketId() {
			return errors.Errorf("object ref different from expected: bucket=%q want %q", o1.GetBucketId(), o2.GetBucketId())
		}
		return nil
	}

	oref1b, _, err := objState.GetRootRef(ctx)
	if err == nil {
		err = assertEqual(oref1b, oref1)
	}
	if err != nil {
		return errors.Wrap(err, "object state get root ref")
	}

	// Commit the initial object transaction.
	err = ws.Commit(ctx)
	if err != nil {
		return err
	}

	// Open a read transaction for persisted-state assertions.
	ws, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer ws.Discard()

	objState, err = world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return errors.Wrapf(err, "get object: %s", objKey)
	}
	var orev1b uint64
	oref1b, orev1b, err = objState.GetRootRef(ctx)
	if err == nil {
		err = assertEqual(oref1b, oref1)
	}
	if err == nil {
		if orev1b != 1 {
			err = errors.Errorf("expected rev 1 just after creating, but got %d", orev1b)
		}
	}
	if err != nil {
		return errors.Wrap(err, "get root ref")
	}

	oref2 := &bucket.ObjectRef{}

	// Confirm that read transactions reject root-reference writes.
	_, err = objState.SetRootRef(ctx, oref2)
	if err != tx.ErrNotWrite {
		return errors.Errorf("expected error %v but got %v", tx.ErrNotWrite, err)
	}

	// Open a writable transaction while retaining the original read snapshot.
	ws2, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// Update the object reference and commit the writable transaction.
	objState2, err := world.MustGetObject(ctx, ws2, objKey)
	if err != nil {
		ws2.Discard()
		return err
	}
	orev, err := objState2.SetRootRef(ctx, oref2)
	if err == nil {
		err = ws2.Commit(ctx)
	}
	if err == nil {
		if orev != 2 {
			err = errors.Errorf("expected rev 2 after writing, but got %d", orev)
		}
	}
	if err != nil {
		ws2.Discard()
		return err
	}

	// Verify whether the original read snapshot observes the committed reference.
	oref1b, _, err = objState.GetRootRef(ctx)
	if err == nil {
		err = assertEqual(oref1b, oref2)
	}
	if err != nil {
		ws3, rerr := eng.NewTransaction(ctx, false)
		if rerr != nil {
			return rerr
		}
		objState3, rerr := world.MustGetObject(ctx, ws3, objKey)
		if rerr == nil {
			var oref3 *bucket.ObjectRef
			oref3, _, rerr = objState3.GetRootRef(ctx)
			if rerr == nil {
				rerr = assertEqual(oref3, oref2)
			}
		}
		ws3.Discard()
		if rerr != nil {
			return rerr
		}
	}

	// Open a writable transaction for graph mutation checks.
	ws2, err = eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// Add a second object and connect it with a graph quad.
	obj2Key := "test-object-2"
	_, err = ws2.CreateObject(ctx, obj2Key, oref1)
	if err != nil {
		ws2.Discard()
		return err
	}

	testQuad1 := world.NewGraphQuad(
		world.KeyToGraphValue(objKey).String(),
		"<parent>",
		world.KeyToGraphValue(obj2Key).String(),
		"",
	)
	err = ws2.SetGraphQuad(ctx, testQuad1)
	if err != nil {
		ws2.Discard()
		return err
	}

	err = ws2.Commit(ctx)
	if err != nil {
		ws2.Discard()
		return err
	}

	// Reopen a read transaction so snapshot-isolated engines observe the graph update.
	ws.Discard()
	ws, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer ws.Discard()

	// Verify that the committed graph quad can be looked up.
	quads, err := ws.LookupGraphQuads(ctx, testQuad1, 1)
	found := len(quads) != 0
	if err == nil && !found {
		err = errors.New("graph quad not found after setting")
	}
	if err != nil {
		return err
	}

	// Exercise a Cayley path query and iterator result handling.
	err = ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		// Build the parent path from the first object key.
		p := cayley.StartPath(h, world.KeyToGraphValue(objKey)).Out(quad.IRI("parent"))

		// Optimize the path and verify its iterator statistics.
		sh, _, err := p.Shape().Optimize(ctx, nil)
		if err != nil {
			return err
		}
		it := sh.BuildIterator(ctx, h)
		stats, err := it.Stats(ctx)
		if err != nil {
			return err
		}
		if stats.Size.Exact && stats.Size.Value != 1 {
			return errors.Errorf("expected size of %d but got %d", 1, stats.Size.Value)
		}

		// Iterate path results and verify the expected child key.
		sc := it.Iterate(ctx)
		defer sc.Close()
		n := 0
		for sc.Next(ctx) {
			ref, err := sc.Result(ctx)
			if err != nil {
				return err
			}
			qv, err := h.NameOf(ctx, ref)
			if err != nil {
				return err
			}
			expected := quad.IRI(obj2Key).String()
			if qvs := qv.String(); qvs != expected {
				return errors.Errorf("expected <parent> to return %s but got %s", expected, qvs)
			}
			n++
		}
		err = sc.Err()
		if err == nil && n != 1 {
			err = errors.Errorf("expected %d result but got %d", 1, n)
		}
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Exercise the parent graph helper against the existing relationship.
	ws2, err = eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	parentStr, err := world_parent.GetObjectParent(ctx, ws2, objKey)
	if err != nil {
		ws2.Discard()
		return err
	}
	if parentStr != obj2Key {
		ws2.Discard()
		return errors.Errorf(
			"expected GetObjectParent(%s) -> %s but got %s",
			objKey, obj2Key, parentStr,
		)
	}
	if err := world_parent.ClearObjectParent(ctx, ws2, objKey); err != nil {
		ws2.Discard()
		return err
	}
	parentStr, err = world_parent.GetObjectParent(ctx, ws2, objKey)
	if err != nil {
		ws2.Discard()
		return err
	}
	if parentStr != "" {
		ws2.Discard()
		return errors.Errorf("expected parent to be empty but got: %s", parentStr)
	}

	// Verify the object type can be stored and read back.
	objTypeID := "mock"
	if err := world_types.SetObjectType(ctx, ws2, objKey, objTypeID); err != nil {
		ws2.Discard()
		return err
	}
	typeStr, err := world_types.GetObjectType(ctx, ws2, objKey)
	if err != nil {
		ws2.Discard()
		return err
	}
	if typeStr != objTypeID {
		ws2.Discard()
		return errors.Errorf(
			"expected GetObjectType(%s) -> %s but got %s",
			objKey, objTypeID, typeStr,
		)
	}
	if err != nil {
		ws2.Discard()
		return err
	}

	err = ws2.Commit(ctx)
	if err != nil {
		return err
	}
	ws.Discard()
	ws, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer ws.Discard()

	// Query the graph for objects carrying the stored type.
	err = ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		p := path.StartPath(h)
		p = world_types.LimitNodesToTypes(p, objTypeID)
		ch := p.Iterate(ctx)
		n, err := ch.Count(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.Errorf("expected 1 object w/ type %q but got %d", objTypeID, n)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Enumerate objects linked to the stored type.
	var objsWithTypeKey []string
	err = world_types.IterateObjectsWithType(ctx, ws, objTypeID, func(objKey string) (bool, error) {
		objsWithTypeKey = append(objsWithTypeKey, objKey)
		return true, nil
	})
	if err != nil {
		return err
	}
	if n := len(objsWithTypeKey); n != 1 {
		return errors.Errorf("expected 1 object w/ type %q but got %d", objTypeID, n)
	}
	if v := objsWithTypeKey[0]; v != objKey {
		return errors.Errorf("expected object %s w/ type %q but got %s", objKey, objTypeID, v)
	}

	// Drive a control loop until repeated operations reach revision 20.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// Configure the control loop's target revision.
	var targetRev uint64 = 20
	loop := world_control.NewWatchLoop(le, objKey, func(
		ctx context.Context,
		le *logrus.Entry,
		ws world.WorldState,
		obj world.ObjectState, // may be nil if not found
		rootRef *bucket.ObjectRef, rev uint64,
	) (bool, error) {
		if obj == nil {
			le.Debug("callback called: object does not exist")
		} else {
			le.Debugf("callback called with rev = %v", rev)
		}

		if rootRef.GetBucketId() != "" {
			rootRef.BucketId = ""
		}
		var prevMsg string

		// _, _, err = world.AccessWorldObject(ctx, ws, objKey, false, func(bcs *block.Cursor) error {
		_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
			eb, err := block.UnmarshalBlock[*block_mock.Example](ctx, bcs, block_mock.NewExampleBlock)
			if err != nil {
				return err
			}
			le.Debugf("at rev = %v message is %q", rev, eb.GetMsg())
			prevMsg = eb.GetMsg()
			return err
		})
		if err != nil {
			return false, err
		}

		nextMsg := "Hello from rev: " + strconv.Itoa(int(rev)) //nolint:gosec
		if rev < targetRev {
			if rev%2 != 0 || prevMsg == "" {
				// Build the next example block for odd revisions.
				eb := block_mock.NewExample(nextMsg)

				// Write the next root block through the object-state access path.
				var changed bool
				_, changed, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
					bcs.SetBlock(eb, true)
					return nil
				})
				if !changed && err == nil {
					err = errors.New("changed = false but expected true")
				}
			} else if rev%10 == 0 {
				// Apply a world operation at revisions divisible by ten.
				_, _, err = ws.ApplyWorldOp(ctx, NewMockWorldOp(objKey, nextMsg), "")
			} else if rev%5 == 0 {
				// Apply an object operation at other selected revisions.
				_, _, err = obj.ApplyObjectOp(ctx, NewMockObjectOp(nextMsg), "")
			} else {
				_, err = obj.IncrementRev(ctx)
			}
			if err != nil {
				return false, err
			}
			return true, nil
		}
		if rev > targetRev {
			return false, errors.Errorf("unexpected exceeded target rev: %v", rev)
		}

		// Stop the loop after the target revision is reached.
		return false, nil
	})

	// Execute the control loop against the engine world state.
	engWs := world.NewEngineWorldState(eng, true)
	if err := loop.Execute(subCtx, engWs); err != nil {
		return err
	}

	// Delete the object after the control-loop assertions complete.
	ws2, err = eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	deleted, err := ws2.DeleteObject(ctx, objKey)
	if err == nil {
		err = ws2.Commit(ctx)
	} else {
		ws2.Discard()
	}
	if err != nil {
		return err
	}
	if !deleted {
		return errors.Errorf("expected deleted %s but got false", objKey)
	}

	blobTestData := []byte("test creating a blob")
	ws2, err = eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// Recreate the object with a blob payload.
	_, bref, err := world.CreateWorldObject(ctx, ws2, objKey, func(bcs *block.Cursor) error {
		_, berr := blob.BuildBlobWithBytes(ctx, blobTestData, bcs)
		return berr
	})
	if err == nil {
		err = ws2.Commit(ctx)
	} else {
		ws2.Discard()
	}
	if err != nil {
		return err
	}

	// Read the blob back and verify its content and unchanged reference.
	le.Infof("stored blob length %d to object %s", len(blobTestData), bref.MarshalString())
	engWs = world.NewEngineWorldState(eng, true)
	var blobReadbackData []byte
	bref2, _, err := world.AccessWorldObject(ctx, engWs, objKey, false, func(bcs *block.Cursor) error {
		var berr error
		blobReadbackData, berr = blob.FetchToBytes(ctx, bcs)
		return berr
	})
	if err == nil {
		if !bytes.Equal(blobReadbackData, blobTestData) {
			err = errors.Errorf("expected data %#v but got %#v", blobTestData, blobReadbackData)
		}
	}
	if err == nil {
		if !bref2.EqualsRef(bref) {
			err = errors.Errorf(
				"expected same object ref because nothing changed but got %v != expected %v",
				bref2.MarshalString(),
				bref.MarshalString(),
			)
		}
	}
	if err != nil {
		return err
	}
	le.Info("read back and verified blob contents from object")

	// Create objects for prefix iteration checks.
	ws2, err = eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// Seed objects under two prefixes.
	testObjs := map[string]*bucket.ObjectRef{
		"test/a":  {BucketId: "test-1"},
		"test/b":  {BucketId: "test-2"},
		"test/c":  {BucketId: "test-3"},
		"other/d": {BucketId: "test-4"},
	}

	for k, ref := range testObjs {
		_, err := ws2.CreateObject(ctx, k, ref)
		if err != nil {
			ws2.Discard()
			return err
		}
	}

	if err := ws2.Commit(ctx); err != nil {
		return err
	}

	// Open a read transaction for forward iteration.
	ws2, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}

	// Verify forward iteration order for the test prefix.
	iter := ws2.IterateObjects(ctx, "test/", false)

	var keys []string
	for iter.Next() {
		if !iter.Valid() {
			iter.Close()
			ws2.Discard()
			return errors.Errorf("iterator invalid during iteration")
		}
		keys = append(keys, iter.Key())
	}
	if err := iter.Err(); err != nil {
		iter.Close()
		ws2.Discard()
		return err
	}
	if len(keys) != 3 {
		iter.Close()
		ws2.Discard()
		return errors.Errorf("forward iteration: expected 3 objects with prefix test/ but got %d", len(keys))
	}
	if keys[0] != "test/a" || keys[1] != "test/b" || keys[2] != "test/c" {
		iter.Close()
		ws2.Discard()
		return errors.Errorf("unexpected forward iteration order: %v", keys)
	}

	iter.Close()
	ws2.Discard()

	// Open a read transaction for reverse iteration.
	ws2, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}

	// Verify reverse iteration order for the test prefix.
	iter = ws2.IterateObjects(ctx, "test/", true)

	keys = nil
	for iter.Next() {
		if !iter.Valid() {
			iter.Close()
			ws2.Discard()
			return errors.Errorf("iterator invalid during reverse iteration")
		}
		keys = append(keys, iter.Key())
	}
	if err := iter.Err(); err != nil {
		iter.Close()
		ws2.Discard()
		return err
	}
	if len(keys) != 3 {
		iter.Close()
		ws2.Discard()
		return errors.Errorf("reverse iteration: expected 3 objects with prefix test/ but got %d", len(keys))
	}
	if keys[0] != "test/c" || keys[1] != "test/b" || keys[2] != "test/a" {
		iter.Close()
		ws2.Discard()
		return errors.Errorf("unexpected reverse iteration order: %v", keys)
	}

	iter.Close()

	// Open a read transaction for seek checks.
	ws2, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}

	// Verify seeking to a prefix key.
	iter = ws2.IterateObjects(ctx, "", false)

	if err := iter.Seek("test/b"); err != nil {
		iter.Close()
		ws2.Discard()
		return err
	}
	if !iter.Valid() {
		iter.Close()
		ws2.Discard()
		return errors.New("iterator invalid after seek")
	}
	if k := iter.Key(); k != "test/b" {
		iter.Close()
		ws2.Discard()
		return errors.Errorf("expected seek to test/b but got %s", k)
	}

	iter.Close()
	ws2.Discard()
	return nil
}
