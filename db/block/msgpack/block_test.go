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
	// Configure the context and logger for the MessagePack round trip.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the block-storage testbed for the MessagePack object.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty cursor for the encoded object.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Prepare a sample object with string and integer fields.
	sampleObj := &testObject{TestField: "testing 123", TestInt: 1337}

	// Encode the sample object into a single MessagePack block.
	// stores the entire object in 1 block always.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(NewMsgpackBlock(sampleObj), true)
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Record the encoded block reference for the decoding transaction.
	blockRef := bcs.GetRef()
	blockRefStr := blockRef.MarshalString()
	le.Infof("encoded to block %s", blockRefStr)

	// Reopen the saved block reference and decode its MessagePack object.
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

	// Verify both decoded fields match the sample object.
	// note: the object should already be written
	if outObj.TestField != sampleObj.TestField || outObj.TestInt != sampleObj.TestInt {
		t.Fatalf("data was different %#v != %#v", outObj, sampleObj)
	}

	// Report the encoded byte count from the saved block.
	rawData, _, _ := bcs.Fetch(ctx)
	t.Logf("successful end-to-end marshal/unmarshal test, len %d bytes", len(rawData))
}

// TestBlockToObject tests block to object and object to block.
func TestBlockToObject(t *testing.T) {
	// Configure the context and logger for the MessagePack round trip.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the block-storage testbed for the MessagePack object.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty cursor for the encoded object.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Prepare a sample object with string and integer fields.
	sampleObj := &testObject{TestField: "testing 123", TestInt: 1337}

	// Encode the sample object into a single MessagePack block.
	// stores the entire object in 1 block always.
	btx, bcs := oc.BuildTransaction(nil)
	err = ObjectToBlock(bcs, sampleObj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Persist the MessagePack block to obtain its saved reference.
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Record the encoded block reference for the decoding transaction.
	blockRef := bcs.GetRef()
	blockRefStr := blockRef.MarshalString()
	le.Infof("encoded to block %s", blockRefStr)

	// Decode the saved block through the BlockToObject interface.
	_, bcs = oc.BuildTransactionAtRef(nil, blockRef)
	var outObj *testObject // alloc location to store address of output
	_, err = BlockToObject(ctx, bcs, &outObj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the decoded string matches the sample object.
	if outObj.TestField != sampleObj.TestField {
		t.Fatalf("object data mismatch: %#v != %#v", sampleObj, outObj)
	}
}
