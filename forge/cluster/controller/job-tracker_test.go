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
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_value "github.com/s4wave/spacewave/forge/value"
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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	controller := NewController(
		tb.Logger,
		tb.Bus,
		NewConfig(tb.EngineID, "test-cluster", tb.Volume.GetPeerID()),
	)
	tracker, _ := controller.jobTrackers.SetKey("test-job", false)

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
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ws := world.NewEngineWorldState(tb.Engine, true)
	obj, _, err := forge_job.CreateJobWithTasks(ctx, ws, tb.Volume.GetPeerID(), "job/completed", nil, tb.Volume.GetPeerID(), timestamp.Now())
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = world.AccessWorldObject(ctx, ws, "job/completed", true, func(cursor *block.Cursor) error {
		job, err := forge_job.UnmarshalJob(ctx, cursor)
		if err != nil {
			return err
		}
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
	obj, err = world.MustGetObject(ctx, ws, "job/completed")
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	root, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Discard()
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
