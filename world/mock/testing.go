package world_mock

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
)

// TestWorldEngine applies all tests to the world engine.
func TestWorldEngine(ctx context.Context, eng world.Engine) error {
	tests := [](func(ctx context.Context, eng world.Engine) error){
		TestWorldEngine_Basic,
	}
	for _, t := range tests {
		err := t(ctx, eng)
		if err != nil {
			return err
		}
	}
	return nil
}

// TestWorldEngine_Basic performs basic sanity tests on a world engine.
func TestWorldEngine_Basic(ctx context.Context, eng world.Engine) error {
	objKey := "test-object"
	// create the object in the world
	ws, err := eng.NewTransaction(true)
	if err != nil {
		return err
	}
	oref1 := &bucket.ObjectRef{BucketId: "test-1"}
	_, err = ws.CreateObject(objKey, oref1)
	if err != nil {
		return err
	}
	// lookup the object
	objState, err := world.MustGetObject(ws, objKey)
	if err != nil {
		return err
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
		return err
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
		return err
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
		return err
	}

	oref2 := &bucket.ObjectRef{BucketId: "testing-2"}

	// expect ErrNotWrite
	var orev uint64
	orev, err = objState.SetRootRef(oref2)
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
	orev, err = objState2.SetRootRef(oref2)
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
		world.KeyToGraphValue(objKey),
		"<parent>",
		world.KeyToGraphValue(obj2Key),
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

	// TODO check a graph query on the original read tx
	return nil
}
