package cluster_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// TestTaskTrackerWakesParentOnFirstComplete verifies that a task tracker wakes
// its parent when the first observed task state is COMPLETE. This models the
// parent job tracker scanning a pending task before the child watcher starts.
func TestTaskTrackerWakesParentOnFirstComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	t.Cleanup(cancel)

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	peerID := tb.Volume.GetPeerID()
	clusterKey := "test-cluster"
	jobKey := "test-job"
	taskName := "task"
	taskKey := forge_job.NewJobTaskKey(jobKey, taskName)

	if _, _, err := forge_cluster.CreateCluster(
		ctx, tb.WorldState, clusterKey, "test-cluster", peerID, peerID,
	); err != nil {
		t.Fatal(err)
	}
	_, _, err = forge_job.CreateJobWithTasks(
		ctx,
		tb.WorldState,
		peerID,
		jobKey,
		map[string]*forge_target.Target{
			taskName: {Exec: &forge_target.Exec{Disable: true}},
		},
		peerID,
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.AssignJobToCluster(
		ctx, tb.WorldState, clusterKey, jobKey, peerID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.StartJob(
		ctx, tb.WorldState, clusterKey, jobKey, peerID,
	); err != nil {
		t.Fatal(err)
	}

	conf := NewConfig(tb.EngineID, clusterKey, peerID)
	controller := NewController(tb.Logger, tb.Bus, conf)
	_, tracker := controller.newJobTracker(jobKey)

	scanned := make(chan struct{}, 1)
	complete := make(chan struct{}, 1)
	tracker.objLoop = world_control.NewWatchLoop(
		tb.Logger,
		jobKey,
		func(
			ctx context.Context,
			le *logrus.Entry,
			ws world.WorldState,
			obj world.ObjectState,
			rootRef *bucket.ObjectRef,
			rev uint64,
		) (bool, error) {
			waitForChanges, err := tracker.processState(ctx, le, ws, obj, rootRef, rev)
			if err != nil {
				return waitForChanges, err
			}
			job, _, err := forge_job.LookupJob(ctx, ws, jobKey)
			if err != nil {
				return waitForChanges, err
			}
			if job.IsComplete() {
				select {
				case complete <- struct{}{}:
				default:
				}
			}
			select {
			case scanned <- struct{}{}:
			default:
			}
			return waitForChanges, nil
		},
	)

	parentDone := make(chan error, 1)
	go func() {
		parentDone <- tracker.objLoop.Execute(ctx, tb.WorldState)
	}()
	select {
	case <-scanned:
	case err := <-parentDone:
		t.Fatalf("parent tracker exited before initial scan: %v", err)
	case <-ctx.Done():
		t.Fatalf("parent tracker did not scan pending task: %v", ctx.Err())
	}

	taskTarget, err := forge_target.LookupTarget(
		ctx, tb.WorldState, forge_task.NewTargetKey(taskKey),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, taskKey, true, func(bcs *block.Cursor) error {
		task, err := forge_task.UnmarshalTask(ctx, bcs)
		if err != nil {
			return err
		}
		task.SetTarget(bcs, taskTarget)
		task.TaskState = forge_task.State_TaskState_COMPLETE
		task.Result = forge_value.NewResultWithSuccess()
		bcs.SetBlock(task, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	tracker.taskTrackers.SetContext(ctx, true)
	select {
	case <-complete:
	case <-ctx.Done():
		t.Fatalf("job did not complete after first COMPLETE task observation: %v", ctx.Err())
	}

	// The cancel below stops the tracker loop and tears the testbed engine down
	// at the same time, so the loop returns whichever it notices first. Both are
	// the shutdown this asserts; anything else is a real error.
	cancel()
	err = <-parentDone
	if !errors.Is(err, context.Canceled) && !errors.Is(err, world_block.ErrEngineClosed) {
		t.Fatalf("parent tracker returned %v after cancellation, want %v or %v", err, context.Canceled, world_block.ErrEngineClosed)
	}
}
