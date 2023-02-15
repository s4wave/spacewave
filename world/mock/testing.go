package world_mock

import (
	"bytes"
	"context"
	"strconv"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/query/path"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
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
	ws, err := eng.NewTransaction(true)
	if err != nil {
		return err
	}
	oref1 := &bucket.ObjectRef{BucketId: "test-1"}
	_, err = ws.CreateObject(objKey, oref1)
	if err != nil {
		return errors.Wrapf(err, "create object: %s", objKey)
	}
	// lookup the object
	objState, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return errors.Wrapf(err, "get object: %s", objKey)
	}

	assertEqual := func(o1, o2 *bucket.ObjectRef) error {
		if !o1.EqualsRef(o2) {
			return errors.New("object ref different from expected")
		}
		return nil
	}

	oref1b, _, err := objState.GetRootRef()
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
	ws, err = eng.NewTransaction(false)
	if err != nil {
		return err
	}
	defer ws.Discard()

	objState, err = world.MustGetObject(ws, objKey)
	if err != nil {
		return errors.Wrapf(err, "get object: %s", objKey)
	}
	var orev1b uint64
	oref1b, orev1b, err = objState.GetRootRef()
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
	_, err = objState.SetRootRef(oref2)
	if err != tx.ErrNotWrite {
		return errors.Errorf("expected error %v but got %v", tx.ErrNotWrite, err)
	}

	// check mechanics of writing while reading
	// this should be possible with a world engine
	ws2, err := eng.NewTransaction(true)
	if err != nil {
		return err
	}

	// update object reference & commit
	objState2, err := world.MustGetObject(ws2, objKey)
	if err != nil {
		ws2.Discard()
		return err
	}
	orev, err := objState2.SetRootRef(oref2)
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
	oref1b, _, err = objState.GetRootRef()
	if err == nil {
		err = assertEqual(oref1b, oref2)
	}
	if err != nil {
		return err
	}

	// test some graph transactions
	ws2, err = eng.NewTransaction(true)
	if err != nil {
		return err
	}

	// add a second object
	obj2Key := "test-object-2"
	_, err = ws2.CreateObject(obj2Key, oref1)
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
	err = ws2.SetGraphQuad(testQuad1)
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
	quads, err := ws.LookupGraphQuads(testQuad1, 1)
	found := len(quads) != 0
	if err == nil && !found {
		err = errors.New("graph quad not found after setting")
	}
	if err != nil {
		return err
	}

	// attempt a cayley graph query
	err = ws.AccessCayleyGraph(false, func(h world.CayleyHandle) error {
		// check obj <parent> -> ?
		p := cayley.StartPath(h, world.KeyToGraphValue(objKey)).Out(quad.IRI("parent"))
		// quad stats + optimization basics
		sh, _ := p.Shape().Optimize(ctx, nil)
		it := sh.BuildIterator(h)
		stats, err := it.Stats(ctx)
		if err != nil {
			return err
		}
		if stats.Size.Exact && stats.Size.Value != 1 {
			return errors.Errorf("expected size of %d but got %d", 1, stats.Size.Value)
		}
		// test iterator basics
		sc := it.Iterate()
		defer sc.Close()
		n := 0
		for sc.Next(ctx) {
			ref := sc.Result()
			qv, err := h.NameOf(ref)
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
	ps := world_parent.NewParentState(ws)
	parentStr, err := ps.GetObjectParent(objKey)
	if err != nil {
		return err
	}
	if parentStr != obj2Key {
		return errors.Errorf(
			"expected GetObjectParent(%s) -> %s but got %s",
			objKey, obj2Key, parentStr,
		)
	}
	if err := ps.ClearObjectParent(ctx, objKey); err != nil {
		return err
	}
	parentStr, err = ps.GetObjectParent(objKey)
	if err != nil {
		return err
	}
	if parentStr != "" {
		return errors.Errorf("expected parent to be empty but got: %s", parentStr)
	}

	// test a type set/lookup
	typeState := world_types.NewTypesState(ctx, ws)
	objTypeKey := "types/mock"
	if err := typeState.SetObjectType(objKey, objTypeKey); err != nil {
		return err
	}
	typeStr, err := typeState.GetObjectType(objKey)
	if err != nil {
		return err
	}
	if typeStr != objTypeKey {
		return errors.Errorf(
			"expected GetObjectType(%s) -> %s but got %s",
			objKey, objTypeKey, typeStr,
		)
	}
	if err != nil {
		return err
	}

	// search for objects with the given type via path
	err = ws.AccessCayleyGraph(false, func(h world.CayleyHandle) error {
		p := path.StartPath(h)
		p = world_types.LimitNodesToTypes(p, objTypeKey)
		ch := p.Iterate(ctx)
		n, err := ch.Count()
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.Errorf("expected 1 object w/ type %q but got %d", objTypeKey, n)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// search for types (iterate references to the type object)
	var objsWithTypeKey []string
	err = typeState.IterateObjectsWithType(objTypeKey, func(objKey string) (bool, error) {
		objsWithTypeKey = append(objsWithTypeKey, objKey)
		return true, nil
	})
	if err != nil {
		return err
	}
	if n := len(objsWithTypeKey); n != 1 {
		return errors.Errorf("expected 1 object w/ type %q but got %d", objTypeKey, n)
	}
	if v := objsWithTypeKey[0]; v != objKey {
		return errors.Errorf("expected object %s w/ type %q but got %s", objKey, objTypeKey, v)
	}

	// test a control loop by applying various operations to increase the
	// revision of an object until the revision >= 20.
	// if any one operation fails, the rev won't increase and the test will fail.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// increment revision until revision >= 20
	var targetRev uint64 = 20
	loop := world_control.NewObjectLoop(le, objKey, func(
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
			eb, err := block.UnmarshalBlock[*block_mock.Example](bcs, block_mock.NewExampleBlock)
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

		nextMsg := "Hello from revision: " + strconv.Itoa(int(rev))
		if rev < targetRev {
			if rev%2 != 0 || prevMsg == "" {
				// odd numbers
				eb := block_mock.NewExample(nextMsg)

				// write next root object into storage
				// note: world.AccessWorldObject is a utility for this
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
				_, _, err = ws.ApplyWorldOp(NewMockWorldOp(objKey, nextMsg), "")
			} else if rev%5 == 0 {
				// even numbers divisible by 5, use object op method
				// note: passing empty peer id
				_, _, err = obj.ApplyObjectOp(NewMockObjectOp(nextMsg), "")
			} else {
				_, err = obj.IncrementRev()
			}
			if err != nil {
				return false, err
			}
			return true, nil
		}
		if rev > targetRev {
			return false, errors.Errorf("unexpected exceeded target revision: %v", rev)
		}
		// stop execution, success
		return false, nil
	})

	// test control loop
	engWs := world.NewEngineWorldState(ctx, eng, true)
	if err := loop.Execute(subCtx, engWs); err != nil {
		return err
	}

	// delete the object
	if ws2 != nil {
		ws2.Discard()
	}
	ws2, err = eng.NewTransaction(true)
	if err != nil {
		return err
	}
	deleted, err := ws2.DeleteObject(objKey)
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
	ws2, err = eng.NewTransaction(true)
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
	engWs = world.NewEngineWorldState(ctx, eng, true)
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

	return nil
}
