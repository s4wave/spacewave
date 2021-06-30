package forge_kvtx

import (
	"bytes"
	"context"
	"testing"

	forge_execution "github.com/aperturerobotics/forge/execution"
	execution_mock "github.com/aperturerobotics/forge/execution/mock"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	kvtx_block "github.com/aperturerobotics/hydra/kvtx/block"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// todo: feed a non-zero size store in

// TestKvtx tests the kvtx execution controller
func TestKvtx(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	testYAML := `
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

	tgt := &target_json.Target{}
	err = tgt.UnmarshalYAML([]byte(testYAML))
	if err != nil {
		t.Fatal(err.Error())
	}

	// ordinarily resolved by Task controller, set it manually
	valueSet := &forge_target.ValueSet{
		Inputs: []*forge_value.Value{{
			Name:      "testValue",
			ValueType: forge_value.ValueType_ValueType_BUCKET_REF,
			// ref set below
		}},
	}
	mockData := []byte("mock blob data 123")
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	_, err = execution_mock.RunTargetInTestbed(
		tb,
		tgt,
		valueSet,
		&execution_mock.RunTargetOpts{
			PreHook: func(s world.WorldState) error {
				vref, err := world.AccessObject(ctx, s.AccessWorldState, nil, func(bcs *block.Cursor) error {
					_, berr := blob.BuildBlobWithBytes(ctx, mockData, bcs)
					return berr
				})
				if err != nil {
					return err
				}
				valueSet.Inputs[0].BucketRef = vref
				_, err = s.CreateObject("testValue", vref)
				return err
			},
			PostHook: func(state world.WorldState, finalState *forge_execution.Execution) error {
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
				h := forge_target.ExecControllerHandleWithAccess(state.AccessWorldState)
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
					return err
				}

				// check setTestValue output
				stv = valMap["setTestValue2"]
				if stv.IsEmpty() {
					t.Fatal("expected setTestValue2 output to be set but was empty")
				}
				mockData2 := []byte("Testing 123")
				h = forge_target.ExecControllerHandleWithAccess(state.AccessWorldState)
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
					return err
				}

				// check setTestValue3 output
				stv = valMap["setTestValue3"]
				if stv.IsEmpty() {
					t.Fatal("expected setTestValue3 output to be set but was empty")
				}
				mv, err := forge_target.LoadMsgpackValue(ctx, h, stv, nil)
				if err != nil {
					return err
				}
				didExist, ok := mv.(bool)
				if !ok {
					return errors.Errorf("expected boolean value but got: %#v", mv)
				}
				if !didExist {
					return errors.New("expected did exist to be true but got false")
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
				return err
			},
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
}
