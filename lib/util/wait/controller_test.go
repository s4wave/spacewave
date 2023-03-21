package forge_lib_util_wait

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	forge_job "github.com/aperturerobotics/forge/job"
	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
	"github.com/pkg/errors"
)

const testYAML = `
inputs:
  - name: example
    inputType: InputType_WORLD_OBJECT
    worldObject:
      objectKey: input/example
      objectRev: 1
outputs:
  - name: example
    outputType: OutputType_EXEC
    execOutput: "example"
exec:
  controller:
    id: forge/lib/util/wait
    config: {}
`

// TestUtilWait tests the wait controller.
//
// Also tests mechanics of waiting for the world object to exist,
func TestUtilWait(t *testing.T) {
	tb, err := testbed.Default(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := tb.Context
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(testYAML))
	if err != nil {
		t.Fatal(err.Error())
	}

	taskMap := map[string]*forge_target.Target{
		"test-wait": tgt,
	}

	var setInputObject atomic.Bool
	go func() {
		<-time.After(time.Second)
		tb.Logger.Info("creating input object for test: input/example")
		_, _, _ = world.CreateWorldObject(ctx, tb.WorldState, "input/example", func(bcs *block.Cursor) error {
			bsl := []byte("hello world!")
			bcs.SetBlock(byteslice.NewByteSlice(&bsl), true)
			return nil
		})
		setInputObject.Store(true)
	}()

	ts := timestamp.Now()
	jobKey := "job/1"
	clusterKey := "cluster/1"
	outState, err := tb.RunWorkerWithTasks(taskMap, nil, 1, &ts, jobKey, clusterKey, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !setInputObject.Load() {
		t.Fatal("expected input object to be set before task completes")
	}
	if outState.GetJobState() != forge_job.State_JobState_COMPLETE {
		t.Fatalf("expected job state COMPLETE but got %s", outState.GetJobState().String())
	}

	// lookup the list of Task for the Job
	jobTasks, _, err := forge_job.CollectJobTasks(ctx, tb.WorldState, jobKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(jobTasks) != 1 {
		t.Fatalf("expected %d job tasks but found %d", 1, len(jobTasks))
	}

	finalState := jobTasks[0]
	outputs := forge_value.ValueSlice(finalState.GetValueSet().GetOutputs())
	valMap, err := outputs.BuildValueMap(true, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check output
	stv := valMap["example"]
	if stv.IsEmpty() {
		t.Fatal("expected example output to be set but was empty")
	}
	if stv.GetValueType() != forge_value.ValueType_ValueType_WORLD_OBJECT_SNAPSHOT {
		t.Fatal(errors.Wrap(forge_target.ErrUnexpectedOutputValueType, stv.GetValueType().String()))
	}
}
