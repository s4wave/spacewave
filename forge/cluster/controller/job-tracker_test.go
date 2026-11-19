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
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestSkipUnhandledOperation(t *testing.T) {
	handler := skipUnhandledOperation(func(
		context.Context,
		*logrus.Entry,
		world.WorldState,
		world.ObjectState,
		*bucket.ObjectRef,
		uint64,
	) (bool, error) {
		return false, world.ErrUnhandledOp
	})

	waitForChanges, err := handler(context.Background(), nil, nil, nil, nil, 0)
	if err != nil {
		t.Fatalf("handler error = %v, want nil", err)
	}
	if !waitForChanges {
		t.Fatal("waitForChanges = false, want true")
	}
}

func TestJobTrackerRetriesTransientWorldError(t *testing.T) {
	// Bound the Job tracker retry test.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	// Open a World testbed for the Job tracker.
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Construct the Cluster controller and its Job tracker.
	controller := NewController(
		tb.Logger,
		tb.Bus,
		NewConfig(tb.EngineID, "test-cluster", tb.Volume.GetPeerID()),
	)
	tracker, _ := controller.jobTrackers.SetKey("test-job", false)

	// Record watch attempts and fail the first World read.
	attempts := make(chan int, 2)
	attempt := 0
	tracker.objLoop = world_control.NewWatchLoop(
		tb.Logger,
		"",
		func(
			context.Context,
			*logrus.Entry,
			world.WorldState,
			world.ObjectState,
			*bucket.ObjectRef,
			uint64,
		) (bool, error) {
			attempt++
			attempts <- attempt
			if attempt == 1 {
				return false, errors.New("transient world read")
			}
			return false, nil
		},
	)

	// Start Job tracking and verify the failed read is retried.
	controller.jobTrackers.SetContext(ctx, true)
	for want := 1; want <= 2; want++ {
		select {
		case got := <-attempts:
			if got != want {
				t.Fatalf("attempt = %d, want %d", got, want)
			}
		case <-ctx.Done():
			t.Fatalf("job tracker attempt %d did not start: %v", want, ctx.Err())
		}
	}
}

// TestCompletedJobReconciliationDoesNotWaitForWriter keeps read-only completion
// replays out of the queue used by foreground World operations.
func TestCompletedJobReconciliationDoesNotWaitForWriter(t *testing.T) {
	// Bound the completed Job replay test.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	// Open a World testbed for completed Job reconciliation.
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Create the Job whose completed state will be replayed.
	ws := world.NewEngineWorldState(tb.Engine, true)
	obj, _, err := forge_job.CreateJobWithTasks(ctx, ws, tb.Volume.GetPeerID(), "job/completed", nil, tb.Volume.GetPeerID(), nil, timestamp.Now())
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Store a successful terminal result on the Job.
	_, _, err = world.AccessWorldObject(ctx, ws, "job/completed", true, func(cursor *block.Cursor) error {
		// Decode the Job before completing it.
		job, err := forge_job.UnmarshalJob(ctx, cursor)
		if err != nil {
			return err
		}

		// Persist the completed Job with its successful result.
		job.JobState = forge_job.State_JobState_COMPLETE
		job.Result = forge_value.NewResultWithSuccess()
		cursor.SetBlock(job, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reconcile the same completed state while a foreground writer is held.
	controller := NewController(tb.Logger, tb.Bus, NewConfig(tb.EngineID, "cluster/test", tb.Volume.GetPeerID()))
	_, tracker := controller.newJobTracker("job/completed")

	// Retain the completed Job snapshot and its revision for replay.
	obj, err = world.MustGetObject(ctx, ws, "job/completed")
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	root, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Hold a foreground writer while completed snapshots are reconciled.
	writer, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()

	// Verify repeated Job reconciliation remains read-only and keeps watching.
	for range 3 {
		wait, err := tracker.processState(ctx, tb.Logger, ws, obj, root, rev)
		if err != nil {
			t.Fatalf("completed replay waited for writer: %v", err)
		}
		if !wait {
			t.Fatal("completed replay stopped watching")
		}
	}
}

// TestJobTrackerAssignsUnclaimedTasks verifies that the Job tracker assigns
// the unclaimed Tasks of a running Job, completes the Job when they complete,
// and leaves a completed unclaimed Task of the completed Job unclaimed
// without failing.
func TestJobTrackerAssignsUnclaimedTasks(t *testing.T) {
	// Bound the test and retain control of tracker shutdown.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	trackerCtx, stopTracker := context.WithCancel(ctx)

	// Open a World testbed with a Cluster.
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	peerID := tb.Volume.GetPeerID()
	clusterKey, jobKey := "test-cluster", "test-job"
	if _, _, err := forge_cluster.CreateCluster(ctx, tb.WorldState, clusterKey, "test-cluster", peerID, peerID); err != nil {
		t.Fatal(err)
	}

	// Create a running Job of two unclaimed Tasks in the Cluster.
	targets := map[string]*forge_target.Target{
		"a": {Exec: &forge_target.Exec{Disable: true}},
		"b": {Exec: &forge_target.Exec{Disable: true}},
	}
	obj, _, err := forge_job.CreateJobWithTasks(ctx, tb.WorldState, peerID, jobKey, targets, peer.ID(""), nil, timestamp.Now())
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.AssignJobToCluster(ctx, tb.WorldState, clusterKey, jobKey, peerID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_cluster.StartJob(ctx, tb.WorldState, clusterKey, jobKey, peerID); err != nil {
		t.Fatal(err)
	}
	taskKeys := []string{forge_job.NewJobTaskKey(jobKey, "a"), forge_job.NewJobTaskKey(jobKey, "b")}

	// Run the Job tracker.
	controller := NewController(tb.Logger, tb.Bus, NewConfig(tb.EngineID, clusterKey, peerID))
	_, tracker := controller.newJobTracker(jobKey)
	done := make(chan error, 1)
	go func() { done <- tracker.objLoop.Execute(trackerCtx, tb.WorldState) }()

	// waitFor waits until cond holds or the test times out.
	waitFor := func(what string, cond func() bool) {
		for !cond() {
			select {
			case err := <-done:
				t.Fatalf("job tracker exited waiting for %s: %v", what, err)
			case <-ctx.Done():
				t.Fatalf("waiting for %s: %v", what, ctx.Err())
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	taskPeer := func(key string) string {
		task, err := forge_task.LookupTaskBody(ctx, tb.WorldState, key)
		if err != nil {
			t.Fatal(err)
		}
		return task.GetPeerId()
	}

	// Both Tasks are assigned to the Cluster.
	waitFor("assignment", func() bool {
		return taskPeer(taskKeys[0]) == peerID.String() && taskPeer(taskKeys[1]) == peerID.String()
	})

	// Complete both Tasks, leaving the second unclaimed, as a Task that fails
	// before any cluster claims it is.
	for i, key := range taskKeys {
		target, err := forge_target.LookupTarget(ctx, tb.WorldState, forge_task.NewTargetKey(key))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := world.AccessWorldObject(ctx, tb.WorldState, key, true, func(bcs *block.Cursor) error {
			task, err := forge_task.UnmarshalTask(ctx, bcs)
			if err != nil {
				return err
			}
			task.SetTarget(bcs, target)
			task.TaskState = forge_task.State_TaskState_COMPLETE
			task.Result = forge_value.NewResultWithSuccess()
			if i == 1 {
				task.PeerId = ""
			}
			bcs.SetBlock(task, true)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	// The tracker completes the Job.
	waitFor("job completion", func() bool {
		job, err := forge_job.LookupJobBody(ctx, tb.WorldState, jobKey)
		if err != nil {
			t.Fatal(err)
		}
		return job.IsComplete()
	})
	stopTracker()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("job tracker returned %v, want %v", err, context.Canceled)
	}

	// Reconciling the completed Job leaves its completed unclaimed Task
	// unclaimed instead of failing to assign it.
	jobObj, err := world.MustGetObject(ctx, tb.WorldState, jobKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(jobObj)
	root, rev, err := jobObj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tracker.processState(ctx, tb.Logger, tb.WorldState, jobObj, root, rev); err != nil {
		t.Fatalf("reconcile completed job: %v", err)
	}
	if got := taskPeer(taskKeys[1]); got != "" {
		t.Fatalf("completed task assigned to %s, want unclaimed", got)
	}
	job, err := forge_job.LookupJobBody(ctx, tb.WorldState, jobKey)
	if err != nil {
		t.Fatal(err)
	}
	if !job.IsComplete() {
		t.Fatalf("completed job reopened to %s", job.GetJobState())
	}
}
