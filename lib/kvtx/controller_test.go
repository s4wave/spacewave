package forge_lib_kvtx

import (
	"bytes"
	"context"
	"testing"

	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
)

const testYAML = `
# note: this test is just for Execution controller.
# the inputs / outputs listed in the Target are not used.
inputs:
  # value to set to test-3 key
  - name: testValue
    inputType: InputType_WORLD_OBJECT
    worldObject:
      objectKey: "test-blob"
outputs:
  # contains the kvtx store modified
  - name: store
    outputType: OutputType_EXEC
    execOutput: "store"
exec:
  controller:
    # rev: 0 -> defaults to 1
    config:
      ops:
      - opType: OpType_SET
        ops:
        - key: "test-1"
          valueString: "Hello World"
        - key: "test-2"
          valueString: "Testing 123"
      - key: "test-2"
        opType: OpType_GET
        output: "setTestValue2"
      - key: "test-2"
        opType: OpType_GET_EXISTS
        output: "setTestValue3"
      - key: "test-2"
        opType: OpType_CHECK
        valueString: "Testing 123"
      - key: "test-4"
        opType: OpType_SET_BLOB
        valueInput: "testValue"
        output: "setTestValue"
      - opType: OpType_CHECK_EXISTS
        key: "test-2"
      - opType: OpType_DELETE
        key: "test-2"
      - opType: OpType_CHECK_NOT_EXISTS
        key: "test-2"
    id: forge/lib/kvtx
`

// todo: feed a non-zero size store in

// TestKvtx tests the kvtx execution controller
func TestKvtx(t *testing.T) {
	tb, err := testbed.Default(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx, le, ws := tb.Context, tb.Logger, tb.WorldState
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(testYAML))
	if err != nil {
		t.Fatal(err.Error())
	}

	// store the blob in a world object
	ts := timestamp.Now()
	uniqueID := "kvtx-test"
	handle := forge_target.ExecControllerHandleWithAccess(uniqueID, tb.Volume.GetPeerID(), tb.Engine, ws.AccessWorldState, ts)
	mockData := []byte("mock blob: hello world")
	testBlob, err := forge_target.StoreBlobValueFromBytes(ctx, handle, mockData)
	if err != nil {
		t.Fatal(err.Error())
	}
	testObj, err := ws.CreateObject(ctx, "test-blob", testBlob.GetBucketRef())
	if err != nil {
		t.Fatal(err.Error())
	}

	valueSet := &forge_target.ValueSet{}

	// TODO: ordinarily resolved by Task controller, set it manually
	// remove this when the Task controller can resolve world objects.
	inpSnapshot, err := forge_value.NewWorldObjectSnapshot(ctx, testObj, ws)
	if err != nil {
		t.Fatal(err.Error())
	}
	inpValue := forge_value.NewValueWithWorldObjectSnapshot("testValue", inpSnapshot)
	valueSet.Inputs = append(valueSet.Inputs, inpValue)

	finalState, err := tb.RunExecutionWithTarget(
		tgt,
		valueSet,
		ts,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	outputs := forge_value.ValueSlice(finalState.GetValueSet().GetOutputs())
	valMap, err := outputs.BuildValueMap(true, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check setTestValue output
	stv := valMap["setTestValue"]
	if stv.IsEmpty() {
		t.Fatal("expected setTestValue output to be set but was empty")
	}
	h := forge_target.ExecControllerHandleWithAccess(uniqueID, tb.Volume.GetPeerID(), tb.Engine, ws.AccessWorldState, ts)
	_, err = forge_target.AccessValue(ctx, h, stv, func(bcs *block.Cursor) error {
		dat, err := blob.FetchToBytes(ctx, bcs)
		if err != nil {
			return err
		}
		if !bytes.Equal(dat, mockData) {
			return errors.Errorf(
				"expected value setTestValue to contain %s but contained %s",
				string(mockData),
				string(dat),
			)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// check setTestValue output
	stv = valMap["setTestValue2"]
	if stv.IsEmpty() {
		t.Fatal("expected setTestValue2 output to be set but was empty")
	}
	mockData2 := []byte("Testing 123")
	h = forge_target.ExecControllerHandleWithAccess(uniqueID, tb.Volume.GetPeerID(), tb.Engine, ws.AccessWorldState, ts)
	_, err = forge_target.AccessValue(ctx, h, stv, func(bcs *block.Cursor) error {
		dat, err := blob.FetchToBytes(ctx, bcs)
		if err != nil {
			return err
		}
		if !bytes.Equal(dat, mockData2) {
			return errors.Errorf(
				"expected value setTestValue2 to contain %s but contained %s",
				string(mockData2),
				string(dat),
			)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// check setTestValue3 output
	stv = valMap["setTestValue3"]
	if stv.IsEmpty() {
		t.Fatal("expected setTestValue3 output to be set but was empty")
	}
	didExist, err := forge_target.LoadMsgpackValue[bool](ctx, h, stv, nil)
	if err == nil && !didExist {
		err = errors.New("expected did exist to be true but got false")
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// check store output
	stv = valMap["store"]
	if stv.IsEmpty() {
		t.Fatal("expected store output to be set but was empty")
	}
	le.Infof("output store reference was: %s", stv.GetBucketRef().MarshalString())
	_, err = forge_target.AccessValue(ctx, h, stv, func(bcs *block.Cursor) error {
		kvs, err := kvtx_block.LoadKeyValueStore(ctx, bcs)
		if err != nil {
			return err
		}
		btx, err := kvs.BuildKvTransaction(ctx, bcs, false)
		if err != nil {
			return err
		}
		nkeys, err := btx.Size(ctx)
		if err != nil {
			return err
		}
		if nkeys != 2 {
			return errors.Errorf("expected %d keys but got %d", 2, nkeys)
		}
		// bug: vbcs is pointing to a node in iavl tree
		tdata, tfound, err := btx.Get(ctx, []byte("test-1"))
		if err != nil {
			err = errors.Wrap(err, "lookup test-1 in access-value")
		}
		if err == nil && !tfound {
			err = errors.New("expected test-1 key to be set but was not")
		}
		if err == nil {
			expected := "Hello World"
			if !bytes.Equal(tdata, []byte(expected)) {
				err = errors.Errorf("expected test-1 key to contain %q but contained %q", expected, string(tdata))
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}
}
