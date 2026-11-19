package cluster_controller

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/sirupsen/logrus"
)

// jobTracker tracks a Job managed by the Cluster.
type jobTracker struct {
	// c is the controller.
	c *Controller
	// objKey is the job object key.
	objKey string
	// objLoop is the object watcher loop.
	objLoop *world_control.WatchLoop
}

// newJobTracker constructs a new job tracker routine.
func (c *Controller) newJobTracker(key string) (keyed.Routine, *jobTracker) {
	tr := &jobTracker{
		c:      c,
		objKey: key,
	}
	tr.objLoop = world_control.NewWatchLoop(
		c.le.WithField("object-loop", "job-tracker"),
		key,
		skipUnhandledOperation(tr.processState),
	)
	return tr.execute, tr
}

// execute executes the job tracker.
func (jt *jobTracker) execute(ctx context.Context) error {
	objKey, le := jt.objKey, jt.c.le

	le.Debugf("starting job tracker: %s", objKey)
	return world_control.ExecuteBusWatchLoop(
		ctx,
		jt.c.bus,
		jt.c.conf.GetEngineId(),
		true,
		jt.objLoop,
	)
}

// skipUnhandledOperation wraps a handler so ErrUnhandledOp waits for changes
// instead of failing the watch loop.
func skipUnhandledOperation(handler world_control.WatchLoopHandler) world_control.WatchLoopHandler {
	return func(
		ctx context.Context,
		le *logrus.Entry,
		ws world.WorldState,
		obj world.ObjectState,
		rootRef *bucket.ObjectRef,
		rev uint64,
	) (bool, error) {
		waitForChanges, err := handler(ctx, le, ws, obj, rootRef, rev)
		if errors.Is(err, world.ErrUnhandledOp) {
			if le != nil {
				le.Debug("job tracker skipped unhandled operation")
			}
			return true, nil
		}
		return waitForChanges, err
	}
}

// processState processes the state for the job.
func (jt *jobTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	// Identify the Job and Cluster for reconciliation.
	jobKey, clusterKey := jt.objKey, jt.c.objKey

	// Confirm the job object type.
	err = forge_job.CheckJobType(ctx, ws, jobKey)
	if err != nil {
		return false, err
	}

	// Decode and validate the job state.
	var job *forge_job.Job
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		job, berr = forge_job.UnmarshalJob(ctx, bcs)
		if berr == nil {
			berr = job.Validate()
		}
		return berr
	})
	if err != nil {
		return true, err
	}

	// Promote pending jobs to running.
	jobState := job.GetJobState()
	if jobState == forge_job.State_JobState_PENDING {
		le.Debugf("starting job: %s", jobKey)
		_, _, err = forge_cluster.StartJob(ctx, ws, clusterKey, jobKey, jt.c.peerID)
		return true, err
	}

	// Cancellation remains terminal until an explicit restart.
	if job.GetResult().GetCanceled() {
		return true, nil
	}

	// Load the tasks linked to the job.
	tasks, taskKeys, err := forge_job.CollectJobTasks(ctx, ws, jobKey)
	if err != nil {
		return true, err
	}

	// Collect the Tasks that still need completion, and the unclaimed ones
	// among them. A Task can complete before any cluster claims it, as when it
	// fails resolving its inputs; it needs no peer.
	var pendingTasks, unclaimedTasks []string
	for i, task := range tasks {
		if task.GetTaskState() == forge_task.State_TaskState_COMPLETE {
			continue
		}
		pendingTasks = append(pendingTasks, taskKeys[i])
		if task.GetPeerId() == "" {
			unclaimedTasks = append(unclaimedTasks, taskKeys[i])
		}
	}

	// Reopen a completed Job with unfinished Tasks: assignment needs a running
	// Job, and the Job's new revision brings this tracker back to assign them.
	le.Debugf("found %d pending tasks: %v", len(pendingTasks), pendingTasks)
	if jobState == forge_job.State_JobState_COMPLETE && len(pendingTasks) != 0 {
		_, _, err = forge_cluster.StartJob(ctx, ws, clusterKey, jobKey, jt.c.peerID)
		return true, err
	}

	// Assign the unclaimed Tasks to this cluster. Tasks another cluster claimed
	// stay with it. The reads above cover every Task body, so a Task that
	// changes, as a retry does, brings this tracker back.
	for _, taskKey := range unclaimedTasks {
		le.WithField("task-key", taskKey).Debug("assigning task to cluster")
		_, _, err = forge_cluster.AssignTaskToCluster(ctx, ws, clusterKey, jobKey, taskKey, jt.c.peerID)
		if err != nil {
			return true, err
		}
	}

	// Reconcile changed results without admitting a writer for completed replays.
	// The operation rechecks current tasks after writer admission.
	result := forge_job.CompletionResult(tasks, taskKeys)
	if result != nil && (!job.IsComplete() || !job.GetResult().EqualVT(result)) {
		jt.c.le.Info("marking job as complete")
		_, _, err = forge_cluster.CompleteJob(ctx, ws, clusterKey, jobKey, jt.c.peerID)
		if err != nil {
			return false, err
		}
	}

	// Keep watching the job for state changes.
	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = (*jobTracker)(nil).processState
