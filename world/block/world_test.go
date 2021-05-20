package world_block

import (
	"context"
	"testing"

	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestWorldState_Basic performs a simple test of operations against world.
func TestWorldState_Basic(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	btx, bcs := ocs.BuildTransaction(nil)
	ws, err := NewWorldState(ctx, btx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// construct a basic example object
	objKey := "test-obj-1"
	objRefCs := ocs.Clone()
	oref := objRefCs.GetRef()
	oref.BucketId = ""
	obtx, obcs := objRefCs.BuildTransaction(nil)
	obcs.SetBlock(block_mock.NewExampleBlock(), true)
	oref.RootRef, obcs, err = obtx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create the object in the world
	_, err = ws.CreateObject(objKey, oref)
	if err != nil {
		t.Fatal(err.Error())
	}

	// lookup the object
	objState, found, err := ws.GetObject(objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatal("expected to find object after create")
	}
	// adjust object ref
	obcs.SetBlock(&block_mock.SubBlock{ExamplePtr: oref.GetRootRef()}, true)
	oref.RootRef, obcs, err = obtx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	// adjust ref in the state
	err = objState.SetRootRef(oref)
	if err != nil {
		t.Fatal(err.Error())
	}

	// commit
	err = ws.Commit()
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
}
