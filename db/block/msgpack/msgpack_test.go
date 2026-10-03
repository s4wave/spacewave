package msgpack

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// testObject tests msgpack encoding.
type testObject struct {
	TestField string `json:"testField"`
	TestInt   int    `json:"testInt"`
}

// TestMsgpackBlob tests a messagepack blob e2e.
func TestMsgpackBlob(t *testing.T) {
	// Configure the context and logger for the MessagePack blob round trip.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the block-storage testbed for the MessagePack blob.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty cursor for the encoded blob.
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Prepare a sample object with string and integer fields.
	sampleObj := &testObject{TestField: "testing 123", TestInt: 1337}

	// Encode the sample object beneath a MsgpackBlob container.
	btx, bcs := oc.BuildTransaction(nil)
	obj, err := BuildMsgpackBlob(ctx, bcs, nil, sampleObj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// obj is the container for the data, stored in bcs as well.
	_ = obj

	// Persist the MsgpackBlob and record its saved block reference.
	_, bcs, err = btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	blockRef := bcs.GetRef()
	blockRefStr := blockRef.MarshalString()
	le.Infof("encoded to block %s", blockRefStr)

	// Reopen the saved block reference and load its MsgpackBlob.
	// decode
	blockRef, err = block.UnmarshalBlockRefB58(blockRefStr)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, bcs = oc.BuildTransactionAtRef(nil, blockRef)
	obj, err = UnmarshalMsgpackBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Decode the blob bytes into the sample object type.
	var outObj *testObject
	err = obj.UnmarshalMsgpack(ctx, bcs, &outObj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify both decoded fields match the sample object.
	if outObj.TestField != sampleObj.TestField || outObj.TestInt != sampleObj.TestInt {
		t.Fatalf("data was different %#v != %#v", outObj, sampleObj)
	}

	// Report the encoded byte count from the saved blob block.
	rawData, _, _ := bcs.Fetch(ctx)
	t.Logf("successful end-to-end marshal/unmarshal test, len %d bytes", len(rawData))
}
