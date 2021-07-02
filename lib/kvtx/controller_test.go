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
	"github.com/pkg/errors"
)

const testYAML = `
# note: this test is just for Execution controller.
# the inputs / outputs listed in the Target are not used.
inputs:
  # value to set to test-3 key
  - name: testValue
    inputType: InputType_WORLD_OBJECT
    objectKey: "testValue"
outputs:
  # contains the kvtx store modified
  - name: store
    outputType: OutputType_EXEC
    execOutput: "store"
exec:
  controller:
    # revision: 0 -> defaults to 1
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
    id: forge/lib/kvtx/1
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

	// ordinarily resolved by Task controller, set it manually
	valueSet := &forge_target.ValueSet{}
	mockData := []byte("mock blob data 123")
	handle := forge_target.ExecControllerHandleWithAccess(ws.AccessWorldState)
	inpValue, err := forge_target.StoreBlobValueFromBytes(ctx, handle, mockData)
	if err != nil {
		t.Fatal(err.Error())
	}
	inpValue.Name = "testValue"
	valueSet.Inputs = append(valueSet.Inputs, inpValue)
	/*
		_, err = ws.CreateObject("testValue", vref)
		if err != nil {
			t.Fatal(err.Error())
		}
	*/
	finalState, err := tb.RunExecutionWithTarget(
		tgt,
		valueSet,
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
	h := forge_target.ExecControllerHandleWithAccess(ws.AccessWorldState)
	_, err = forge_target.AccessValue(ctx, h, stv, func(bcs *block.Cursor) error {
		dat, err := blob.FetchToBytes(ctx, bcs)
		if err != nil {
			return err
		}
		if bytes.Compare(dat, mockData) != 0 {
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
	h = forge_target.ExecControllerHandleWithAccess(ws.AccessWorldState)
	_, err = forge_target.AccessValue(ctx, h, stv, func(bcs *block.Cursor) error {
		dat, err := blob.FetchToBytes(ctx, bcs)
		if err != nil {
			return err
		}
		if bytes.Compare(dat, mockData2) != 0 {
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
	mv, err := forge_target.LoadMsgpackValue(ctx, h, stv, nil)
	didExist, ok := mv.(bool)
	if err == nil && !ok {
		err = errors.Errorf("expected boolean value but got: %#v", mv)
	}
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
		kvs, err := kvtx_block.LoadKeyValueStore(bcs)
		if err != nil {
			return err
		}
		btx, err := kvs.BuildKvTransaction(ctx, bcs, false)
		if err != nil {
			return err
		}
		nkeys, err := btx.Size()
		if err != nil {
			return err
		}
		if nkeys != 2 {
			return errors.Errorf("expected %d keys but got %d", 2, nkeys)
		}
		// bug: vbcs is pointing to a node in iavl tree
		tdata, tfound, err := btx.Get([]byte("test-1"))
		if err != nil {
			err = errors.Wrap(err, "lookup test-1 in access-value")
		}
		if err == nil && !tfound {
			err = errors.New("expected test-1 key to be set but was not")
		}
		if err == nil {
			expected := "Hello World"
			if bytes.Compare(tdata, []byte(expected)) != 0 {
				err = errors.Errorf("expected test-1 key to contain %q but contained %q", expected, string(tdata))
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err.Error())
	}
}
