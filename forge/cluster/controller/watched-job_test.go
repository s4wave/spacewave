package cluster_controller_test

import (
	"context"
	"errors"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_value "github.com/s4wave/spacewave/forge/value"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
)

// TestWatchedJobRestartsAfterWorkerReconnect exercises completion, a worker
// restart, and two later source revisions through the real Forge controllers.
func TestWatchedJobRestartsAfterWorkerReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	worker, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Keep source creation and mutation on the same World API used by inputs.
	writeSource := func(value string) {
		t.Helper()
		body := []byte(value)
		_, _, err := world.AccessWorldObject(ctx, tb.WorldState, "source", true, func(cursor *block.Cursor) error {
			cursor.SetBlock(byteslice.NewByteSlice(&body), true)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	writeSource("first")
	_, _, err = world.AccessWorldObject(ctx, tb.WorldState, "gate", true, func(cursor *block.Cursor) error {
		body := []byte("ready")
		cursor.SetBlock(byteslice.NewByteSlice(&body), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	target := &forge_target.Target{
		Inputs: []*forge_target.Input{{
			Name: "source", InputType: forge_target.InputType_InputType_WORLD_OBJECT,
			WatchChanges: true, WorldObject: &forge_target.InputWorldObject{ObjectKey: "source", ObjectRev: 1},
		}, {
			Name: "gate", InputType: forge_target.InputType_InputType_WORLD_OBJECT,
			WatchChanges: true, WorldObject: &forge_target.InputWorldObject{ObjectKey: "gate", ObjectRev: 1},
		}},
		Exec: &forge_target.Exec{Disable: true},
	}
	once := target.CloneVT()
	for _, input := range once.Inputs {
		input.WatchChanges = false
	}
	job, err := tb.RunWorkerWithTasks(map[string]*forge_target.Target{"project": target, "once": once}, nil, 1, timestamp.Now(), "job", "cluster", worker)
	if err != nil {
		t.Fatal(err)
	}
	initialOnce, err := forge_task.LookupTaskBody(ctx, tb.WorldState, forge_job.NewJobTaskKey("job", "once"))
	if err != nil {
		t.Fatal(err)
	}
	if !job.GetResult().GetSuccess() {
		t.Fatal("initial job did not succeed")
	}
	taskKey := forge_job.NewJobTaskKey("job", "project")
	initial, err := forge_task.LookupTaskBody(ctx, tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}

	// The first worker has stopped. A new controller must recover watched
	// inputs from a completed Task without outside retry or wake commands.
	opRelease, err := tb.Bus.AddController(ctx, world.NewLookupOpController("watched-job-ops", tb.EngineID, forge_world.LookupWorldOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer opRelease()
	peerRelease, err := tb.Bus.AddController(ctx, peer_controller.NewController(tb.Logger, worker), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peerRelease()
	_, workerRef, err := worker_controller.StartControllerWithConfig(ctx, tb.Bus, worker_controller.NewConfig(tb.EngineID, "worker/1", worker.GetPeerID(), true))
	if err != nil {
		t.Fatal(err)
	}
	defer workerRef.Release()

	for index, value := range []string{"second", "third"} {
		if _, err := tb.WorldState.DeleteObject(ctx, "gate"); err != nil {
			t.Fatal(err)
		}
		writeSource(value)
		// Missing a required watched input keeps the restarted Task pending,
		// making the Job's transition back to running directly observable.
		jobLoop := world_control.NewWatchLoop(tb.Logger, "job", world_control.NewWaitForStateHandler(func(ctx context.Context, _ world.WorldState, _ world.ObjectState, cursor *block.Cursor, _ uint64) (bool, error) {
			job, err := forge_job.UnmarshalJob(ctx, cursor)
			if err == nil && job.GetJobState() == forge_job.State_JobState_RUNNING && job.GetResult() != nil {
				t.Error("restarted Job retained its previous terminal result")
			}
			return job.GetJobState() != forge_job.State_JobState_RUNNING, err
		}))
		if err := jobLoop.Execute(ctx, tb.WorldState); err != nil {
			t.Fatalf("job did not return to running: %v", err)
		}
		_, _, err = world.AccessWorldObject(ctx, tb.WorldState, "gate", true, func(cursor *block.Cursor) error {
			body := []byte("ready")
			cursor.SetBlock(byteslice.NewByteSlice(&body), true)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		expectedNonce := initial.GetPassNonce() + uint64(index) + 1
		loop := world_control.NewWatchLoop(tb.Logger, taskKey, world_control.NewWaitForStateHandler(func(ctx context.Context, state world.WorldState, object world.ObjectState, cursor *block.Cursor, _ uint64) (bool, error) {
			task, err := forge_task.UnmarshalTask(ctx, cursor)
			if err != nil {
				return false, err
			}
			return task.GetTaskState() != forge_task.State_TaskState_COMPLETE || task.GetPassNonce() < expectedNonce, nil
		}))
		if err := loop.Execute(ctx, tb.WorldState); err != nil {
			t.Fatalf("source %q did not finish another Pass: %v", value, err)
		}
		job, err = forge_job.WaitJobComplete(ctx, tb.Logger, tb.WorldState, "job")
		if err != nil || !job.GetResult().GetSuccess() {
			t.Fatalf("source %q job completion: %v, %v", value, job, err)
		}
		finishedOnce, err := forge_task.LookupTaskBody(ctx, tb.WorldState, forge_job.NewJobTaskKey("job", "once"))
		if err != nil || finishedOnce.GetPassNonce() != initialOnce.GetPassNonce() {
			t.Fatalf("unwatched Task restarted: %v, %v", finishedOnce, err)
		}
	}

	// Reproduce a coalesced fast retry: the next observed Task is also
	// COMPLETE but has a different result. The Job must not keep old success.
	_, _, err = world.AccessWorldObject(ctx, tb.WorldState, taskKey, true, func(cursor *block.Cursor) error {
		task, err := forge_task.UnmarshalTask(ctx, cursor)
		if err != nil {
			return err
		}
		task.Result = forge_value.NewResultWithError(errors.New("example failure"))
		cursor.SetBlock(task, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	loop := world_control.NewWatchLoop(tb.Logger, "job", world_control.NewWaitForStateHandler(func(ctx context.Context, _ world.WorldState, _ world.ObjectState, cursor *block.Cursor, _ uint64) (bool, error) {
		job, err := forge_job.UnmarshalJob(ctx, cursor)
		return !job.IsComplete() || job.GetResult().GetSuccess(), err
	}))
	if err := loop.Execute(ctx, tb.WorldState); err != nil {
		t.Fatalf("Job retained success after a changed Task result: %v", err)
	}
}
