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
	// Bound the tracker test and retain control of tracker shutdown.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	trackerCtx, stopTrackers := context.WithCancel(ctx)

	// Open a World testbed for the Cluster and its Job.
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Identify the Cluster, Job, and Task records used by the test.
	peerID := tb.Volume.GetPeerID()
	clusterKey := "test-cluster"
	jobKey := "test-job"
	taskName := "task"
	taskKey := forge_job.NewJobTaskKey(jobKey, taskName)

	// Create the Cluster that will receive the Job.
	if _, _, err := forge_cluster.CreateCluster(
		ctx, tb.WorldState, clusterKey, "test-cluster", peerID, peerID,
	); err != nil {
		t.Fatal(err)
	}

	// Create a Job with one disabled Task ready for assignment.
	var createdObject world.ObjectState
	createdObject, _, err = forge_job.CreateJobWithTasks(
		ctx,
		tb.WorldState,
		peerID,
		jobKey,
		map[string]*forge_target.Target{
			taskName: {Exec: &forge_target.Exec{Disable: true}},
		},
		peerID,
		nil,
		timestamp.Now(),
	)
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Assign the Job to the Cluster and begin its execution.
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

	// Construct the Job tracker that will observe Task completion.
	conf := NewConfig(tb.EngineID, clusterKey, peerID)
	controller := NewController(tb.Logger, tb.Bus, conf)
	_, tracker := controller.newJobTracker(jobKey)

	// Observe parent scans and completed Job snapshots.
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
			// Reconcile the Job before observing its completion.
			waitForChanges, err := tracker.processState(ctx, le, ws, obj, rootRef, rev)
			if err != nil {
				return waitForChanges, err
			}

			// Read the reconciled Job and signal its terminal state.
			job, objectState, err := forge_job.LookupJob(ctx, ws, jobKey)
			world.ReleaseObjectState(objectState)
			if err != nil {
				return waitForChanges, err
			}
			if job.IsComplete() {
				select {
				case complete <- struct{}{}:
				default:
				}
			}

			// Signal that the parent finished scanning the Job.
			select {
			case scanned <- struct{}{}:
			default:
			}

			return waitForChanges, nil
		},
	)

	// Run the parent tracker until its first pending Task scan completes.
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

	// Fail the Task tracker first read to exercise its retry behavior.
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

	// Load the Task target needed to write a valid completed snapshot.
	taskTarget, err := forge_target.LookupTarget(
		ctx, tb.WorldState, forge_task.NewTargetKey(taskKey),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Complete the Task before its tracker observes its first successful state.
	if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, taskKey, true, func(bcs *block.Cursor) error {
		// Decode the Task whose completed snapshot will wake the parent.
		task, err := forge_task.UnmarshalTask(ctx, bcs)
		if err != nil {
			return err
		}

		// Store a successful completed Task with its resolved target.
		task.SetTarget(bcs, taskTarget)
		task.TaskState = forge_task.State_TaskState_COMPLETE
		task.Result = forge_value.NewResultWithSuccess()
		bcs.SetBlock(task, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Start Task tracking and require the retried observation to complete the Job.
	tracker.taskTrackers.SetContext(trackerCtx, true)
	select {
	case <-complete:
	case <-ctx.Done():
		t.Fatalf("job did not complete after task tracker retry: %v", ctx.Err())
	}

	// Cancel the trackers and verify the parent reports cancellation.
	stopTrackers()
	err = <-parentDone
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("parent tracker returned %v after cancellation, want %v", err, context.Canceled)
	}
}
