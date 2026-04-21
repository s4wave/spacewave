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
	// create the object in the world
	ws, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	oref1 := &bucket.ObjectRef{BucketId: "test-1"}
	_, err = ws.CreateObject(ctx, objKey, oref1)
	if err != nil {
		return errors.Wrapf(err, "create object: %s", objKey)
	}
	// lookup the object
	objState, err := world.MustGetObject(ctx, ws, objKey)
	if err != nil {
		return errors.Wrapf(err, "get object: %s", objKey)
	}

	assertEqual := func(o1, o2 *bucket.ObjectRef) error {
		if !o1.EqualsRef(o2) {
			return errors.New("object ref different from expected")
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

	// commit
	err = ws.Commit(ctx)
	if err != nil {
		return err
	}

	// create read tx
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

	oref2 := &bucket.ObjectRef{BucketId: "testing-2"}

	// expect ErrNotWrite
	_, err = objState.SetRootRef(ctx, oref2)
	if err != tx.ErrNotWrite {
		return errors.Errorf("expected error %v but got %v", tx.ErrNotWrite, err)
	}

	// check mechanics of writing while reading
	// this should be possible with a world engine
	ws2, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// update object reference & commit
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

	// check if original read tx was updated (we expect yes)
	oref1b, _, err = objState.GetRootRef(ctx)
	if err == nil {
		err = assertEqual(oref1b, oref2)
	}
	if err != nil {
		return err
	}

	// test some graph transactions
	ws2, err = eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// add a second object
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

	// check quad exists on original read tx
	quads, err := ws.LookupGraphQuads(ctx, testQuad1, 1)
	found := len(quads) != 0
	if err == nil && !found {
		err = errors.New("graph quad not found after setting")
	}
	if err != nil {
		return err
	}

	// attempt a cayley graph query
	err = ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		// check obj <parent> -> ?
		p := cayley.StartPath(h, world.KeyToGraphValue(objKey)).Out(quad.IRI("parent"))
		// quad stats + optimization basics
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
		// test iterator basics
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

	// attempt a parent graph system query using our existing <parent> quad
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

	// test a type set/lookup
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

	// search for objects with the given type via path
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

	// search for types (iterate references to the type object)
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

	// test a control loop by applying various operations to increase the
	// revision of an object until the revision >= 20.
	// if any one operation fails, the rev won't increase and the test will fail.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// increment revision until revision >= 20
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
				// odd numbers
				eb := block_mock.NewExample(nextMsg)

				// write next root object into storage
				// note: world.AccessObjectState is a utility for this
				// var nroot *bucket.ObjectRef
				var changed bool
				_, changed, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
					bcs.SetBlock(eb, true)
					return nil
				})
				if !changed && err == nil {
					err = errors.New("changed = false but expected true")
				}
			} else if rev%10 == 0 {
				// even numbers divisible by 10, use world op method
				_, _, err = ws.ApplyWorldOp(ctx, NewMockWorldOp(objKey, nextMsg), "")
			} else if rev%5 == 0 {
				// even numbers divisible by 5, use object op method
				// note: passing empty peer id
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
		// stop execution, success
		return false, nil
	})

	// test control loop
	engWs := world.NewEngineWorldState(eng, true)
	if err := loop.Execute(subCtx, engWs); err != nil {
		return err
	}

	// delete the object
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
	// test access object to create a blob
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

	// read the data out again
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

	// Test IterateObjects
	ws2, err = eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	// Create a few test objects with different prefixes
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

	// Create new transaction for iteration tests
	ws2, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}

	// Test forward iteration with prefix
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

	// Create new transaction for reverse iteration
	ws2, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}

	// Test reverse iteration with prefix
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

	// Create new transaction for seek test
	ws2, err = eng.NewTransaction(ctx, false)
	if err != nil {
		return err
	}

	// Test seek
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
