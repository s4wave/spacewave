package task_controller_test

import (
	"context"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_controller "github.com/s4wave/spacewave/forge/task/controller"
	forge_testbed "github.com/s4wave/spacewave/forge/testbed"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// TestTaskOutputWatchesCompletionAndRerun exercises the Task controller's source watch.
func TestTaskOutputWatchesCompletionAndRerun(t *testing.T) {
	// Open the production Forge bus and World with a bounded test lifetime.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	tb, err := forge_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Create a pending source and a reader whose input follows its output.
	sourceKey, readerKey := "test/task/source", "test/task/reader"
	sender := tb.Volume.GetPeerID()
	input := &forge_target.Input{
		Name:         "selected",
		InputType:    forge_target.InputType_InputType_TASK_OUTPUT,
		WatchChanges: true,
		TaskOutput:   &forge_target.InputTaskOutput{TaskKey: sourceKey, OutputName: "present"},
	}
	target := &forge_target.Target{
		Inputs: []*forge_target.Input{input},
		Exec:   &forge_target.Exec{Disable: true},
	}
	for key, tgt := range map[string]*forge_target.Target{
		sourceKey: {Exec: &forge_target.Exec{Disable: true}},
		readerKey: target,
	} {
		obj, _, err := forge_task.CreateTaskWithTarget(ctx, tb.WorldState, sender,
			key, "task-output", tgt, sender, 1, nil, timestamp.Now())
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Resolve missing and pending source outputs as unresolved inputs.
	inputWorld := forge_target.NewInputValueWorld(tb.EngineID, nil, tb.WorldState)
	for _, key := range []string{"test/task/missing", sourceKey} {
		input.TaskOutput.TaskKey = key
		inputs, unset, release, err := forge_target.ResolveInputMap(ctx, tb.Bus, inputWorld, target, nil, forge_task.ResolveOutput)
		if err != nil {
			t.Fatal(err)
		}
		release()
		if len(inputs) != 0 || len(unset) != 1 {
			t.Fatalf("source %q: inputs %v, unresolved %v", key, inputs, unset)
		}
	}
	input.TaskOutput.TaskKey = sourceKey

	// Attach the reader controller while its source still has no output.
	_, ref, err := task_controller.StartControllerWithConfig(ctx, tb.Bus,
		task_controller.NewConfig(tb.EngineID, readerKey, sender, true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)

	// Complete the source fixture with a real named output reference.
	obj, found, err := tb.WorldState.GetObject(ctx, sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("source fixture disappeared")
	}

	// Build the source's snapshot before releasing its object handle.
	snapshot, err := forge_value.NewWorldObjectSnapshot(ctx, obj, tb.WorldState)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the completed source and await its reader's admitted Pass.
	output := forge_value.NewValueWithWorldObjectSnapshot("present", snapshot)
	setTaskOutputFixture(t, ctx, tb.WorldState, sourceKey, forge_task.State_TaskState_COMPLETE, []*forge_value.Value{output})
	reader := waitTaskOutputState(t, ctx, tb.WorldState, readerKey, forge_task.State_TaskState_RUNNING)
	if got := reader.GetValueSet().GetInputs(); len(got) != 1 || got[0].GetName() != "selected" || !got[0].GetWorldObjectSnapshot().EqualVT(output.GetWorldObjectSnapshot()) {
		t.Fatalf("reader inputs = %v, want renamed source output", got)
	}

	// Record a completed reader fixture, then rerun its source with stale outputs retained.
	setTaskOutputFixture(t, ctx, tb.WorldState, readerKey, forge_task.State_TaskState_COMPLETE, nil)
	setTaskOutputFixture(t, ctx, tb.WorldState, sourceKey, forge_task.State_TaskState_PENDING, []*forge_value.Value{output})
	reader = waitTaskOutputState(t, ctx, tb.WorldState, readerKey, forge_task.State_TaskState_PENDING)
	if len(reader.GetValueSet().GetInputs()) != 0 {
		t.Fatal("pending source retained the reader's stale input")
	}
}

// setTaskOutputFixture stores a source or reader state through the World storage interface.
func setTaskOutputFixture(t *testing.T, ctx context.Context, ws world.WorldState, key string, state forge_task.State, outputs []*forge_value.Value) {
	t.Helper()
	_, _, err := world.AccessWorldObject(ctx, ws, key, true, func(cursor *block.Cursor) error {
		// Read the fixture Task before replacing its state and outputs.
		task, err := forge_task.UnmarshalTask(ctx, cursor)
		if err != nil {
			return err
		}

		// Preserve resolved inputs while recording the fixture's observed state.
		task.TaskState = state
		if task.ValueSet == nil {
			task.ValueSet = forge_target.NewValueSet()
		}
		task.ValueSet.Outputs = outputs
		cursor.SetBlock(task, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// waitTaskOutputState waits on World revisions until the reader reaches the requested state.
func waitTaskOutputState(t *testing.T, ctx context.Context, ws world.WorldState, key string, state forge_task.State) *forge_task.Task {
	t.Helper()
	for {
		// Capture a World revision before reading the Task so no write is missed.
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		task, err := forge_task.LookupTaskBody(ctx, ws, key)
		if err != nil {
			t.Fatal(err)
		}
		if task.GetTaskState() == state {
			return task
		}

		// Await the next committed World change without polling.
		if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatalf("waiting for %s, last state %s: %v", state, task.GetTaskState(), err)
		}
	}
}
