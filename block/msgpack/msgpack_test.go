package msgpack

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// testObject tests msgpack encoding.
type testObject struct {
	TestField string `json:"testField"`
	TestInt   int    `json:"testInt"`
}

// TestMsgpackBlob tests a messagepack blob e2e.
func TestMsgpackBlob(t *testing.T) {
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

	btx, bcs := oc.BuildTransaction(nil)
	obj, err := BuildMsgpackBlob(ctx, bcs, nil, sampleObj)
	if err != nil {
		t.Fatal(err.Error())
	}
	// obj is the container for the data, stored in bcs as well.
	_ = obj
	_, bcs, err = btx.Write(true)
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
	obj, err = UnmarshalMsgpackBlob(ctx, bcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	var outObj *testObject
	err = obj.UnmarshalMsgpack(ctx, bcs, &outObj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if outObj.TestField != sampleObj.TestField || outObj.TestInt != sampleObj.TestInt {
		t.Fatalf("data was different %#v != %#v", outObj, sampleObj)
	}
	rawData, _, _ := bcs.Fetch(ctx)
	t.Logf("successful end-to-end marshal/unmarshal test, len %d bytes", len(rawData))
}
