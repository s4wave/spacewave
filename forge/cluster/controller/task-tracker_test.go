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
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// TestTaskTrackerRetriesTransientWorldError verifies that a task tracker
// retries after a World error and wakes its parent when the first task state it
// observes is COMPLETE.
func TestTaskTrackerRetriesTransientWorldError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	trackerCtx, stopTrackers := context.WithCancel(ctx)

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
		parentDone <- tracker.objLoop.Execute(trackerCtx, tb.WorldState)
	}()
	select {
	case <-scanned:
	case err := <-parentDone:
		t.Fatalf("parent tracker exited before initial scan: %v", err)
	case <-ctx.Done():
		t.Fatalf("parent tracker did not scan pending task: %v", ctx.Err())
	}

	taskTracker, _ := tracker.taskTrackers.SetKey(taskKey, false)
	attempt := 0
	taskTracker.objLoop = world_control.NewWatchLoop(
		tb.Logger,
		taskKey,
		func(
			ctx context.Context,
			le *logrus.Entry,
			ws world.WorldState,
			obj world.ObjectState,
			rootRef *bucket.ObjectRef,
			rev uint64,
		) (bool, error) {
			attempt++
			if attempt == 1 {
				return false, errors.New("transient world read")
			}
			return taskTracker.processState(ctx, le, ws, obj, rootRef, rev)
		},
	)

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

	tracker.taskTrackers.SetContext(trackerCtx, true)
	select {
	case <-complete:
	case <-ctx.Done():
		t.Fatalf("job did not complete after task tracker retry: %v", ctx.Err())
	}

	stopTrackers()
	err = <-parentDone
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("parent tracker returned %v after cancellation, want %v", err, context.Canceled)
	}
}
