package msgpack

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestMsgpackBlock tests a messagepack block e2e.
func TestMsgpackBlock(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	sampleObj := &testObject{TestField: "testing 123", TestInt: 1337}

	// stores the entire object in 1 block always.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewMsgpackBlock(sampleObj), true)
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	blockRef := bcs.GetRef()
	blockRefStr := blockRef.MarshalString()
	le.Infof("encoded to block %s", blockRefStr)

	// decode
	blockRef, err = block.UnmarshalBlockRefB58(blockRefStr)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs = oc.BuildTransactionAtRef(nil, blockRef)
	ublk, err := UnmarshalMsgpackBlock(ctx, bcs, func() *testObject {
		return &testObject{}
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	outObj := ublk.GetObj()
	// note: the object should already be written
	if outObj.TestField != sampleObj.TestField || outObj.TestInt != sampleObj.TestInt {
		t.Fatalf("data was different %#v != %#v", outObj, sampleObj)
	}
	rawData, _, _ := bcs.Fetch(ctx)
	t.Logf("successful end-to-end marshal/unmarshal test, len %d bytes", len(rawData))
}

// TestBlockToObject tests block to object and object to block.
func TestBlockToObject(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	sampleObj := &testObject{TestField: "testing 123", TestInt: 1337}

	// stores the entire object in 1 block always.
	btx, bcs := oc.BuildTransaction(nil)
	err = ObjectToBlock(bcs, sampleObj)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	blockRef := bcs.GetRef()
	blockRefStr := blockRef.MarshalString()
	le.Infof("encoded to block %s", blockRefStr)

	_, bcs = oc.BuildTransactionAtRef(nil, blockRef)
	var outObj *testObject // alloc location to store address of output
	_, err = BlockToObject(ctx, bcs, &outObj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if outObj.TestField != sampleObj.TestField {
		t.Fatalf("object data mismatch: %#v != %#v", sampleObj, outObj)
	}
}
