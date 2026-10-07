package forge_lib_util_presence

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// TestPresenceOutcomes runs the presence Target on a Worker for an existing
// and a missing path, and requires exactly the matching output.
func TestPresenceOutcomes(t *testing.T) {
	// Create one existing path beside one missing path.
	dir := t.TempDir()
	present := filepath.Join(dir, "flag")
	if err := os.WriteFile(present, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for want, path := range map[string]string{
		"found":   present,
		"missing": filepath.Join(dir, "absent"),
	} {
		t.Run(want, func(t *testing.T) {
			outputs := runPresence(t, path)
			if len(outputs) != 1 || outputs[0].GetName() != want {
				t.Fatalf("outputs = %v, want only %s", outputs, want)
			}
		})
	}
}

// runPresence runs one presence Task to completion on a testbed Worker and
// returns its outputs after checking that each holds the path.
func runPresence(t *testing.T, path string) forge_value.ValueSlice {
	// Start a Forge testbed with the presence controller factory.
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	// Declare both ports as EXEC outputs of the Target.
	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(`
outputs:
  - name: found
    outputType: OutputType_EXEC
    execOutput: found
  - name: missing
    outputType: OutputType_EXEC
    execOutput: missing
exec:
  controller:
    id: forge/lib/util/presence
    config:
      path: `+strconv.Quote(path)+`
      presentOutput: found
      absentOutput: missing
`))
	if err != nil {
		t.Fatal(err)
	}

	// Run the Task on a Worker until its Job completes.
	taskMap := map[string]*forge_target.Target{"presence": tgt}
	jobKey := "job/1"
	job, err := tb.RunWorkerWithTasks(taskMap, nil, 1, timestamp.Now(), jobKey, "cluster/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if job.GetJobState() != forge_job.State_JobState_COMPLETE {
		t.Fatalf("job state = %s", job.GetJobState())
	}

	// Read the completed Task's outputs.
	tasks, _, err := forge_job.CollectJobTasks(ctx, tb.WorldState, jobKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("job has %d tasks", len(tasks))
	}

	// The Task lists every declared output; an unset one stays empty.
	var outputs forge_value.ValueSlice
	for _, output := range tasks[0].GetValueSet().GetOutputs() {
		if !output.IsEmpty() {
			outputs = append(outputs, output)
		}
	}

	// Require each set output to hold the checked path.
	handle := forge_target.ExecControllerHandleWithAccess("presence-test", "", tb.Engine, tb.WorldState.AccessWorldState, nil)
	for _, output := range outputs {
		if output.GetValueType() != forge_value.ValueType_ValueType_BLOCK_REF {
			t.Fatalf("output %s type = %s", output.GetName(), output.GetValueType())
		}
		data, err := forge_target.LoadBlobValueToBytes(ctx, handle, output)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != path {
			t.Fatalf("output %s holds %q, want %q", output.GetName(), data, path)
		}
	}
	return outputs
}
