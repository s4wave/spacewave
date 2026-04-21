package cluster_controller

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/sirupsen/logrus"
)

// taskTracker tracks a Task managed by the Cluster.
type taskTracker struct {
	// jt is the job tracker
	jt *jobTracker
	// objKey is the task object key
	objKey string
	// objLoop is the object watcher loop
	objLoop *world_control.WatchLoop
	// prevState is the prev task state
	prevState forge_task.State
}

// newTaskTracker constructs a new task tracker routine.
func (jt *jobTracker) newTaskTracker(key string) (keyed.Routine, *taskTracker) {
	tr := &taskTracker{
		jt:     jt,
		objKey: key,
	}
	tr.objLoop = world_control.NewWatchLoop(
		jt.c.le.WithField("object-loop", "task-tracker"),
		key,
		tr.processState,
	)
	return tr.execute, tr
}

// execute executes the job tracker.
func (t *taskTracker) execute(ctx context.Context) error {
	objKey, le := t.objKey, t.jt.c.le

	le.Debugf("job %s: starting task tracker: %s", t.jt.objKey, objKey)
	return world_control.ExecuteBusWatchLoop(
		ctx,
		t.jt.c.bus,
		t.jt.c.conf.GetEngineId(),
		true,
		t.objLoop,
	)
}

// processState processes the state for the job.
func (t *taskTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	taskKey, jobKey, clusterKey := t.objKey, t.jt.objKey, t.jt.c.objKey

	// check the <type> of the task object
	err = forge_task.CheckTaskType(ctx, ws, taskKey)
	if err != nil {
		return false, err
	}

	// unmarshal Task state
	var task *forge_task.Task
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		task, berr = forge_task.UnmarshalTask(ctx, bcs)
		if berr == nil {
			berr = task.Validate()
		}
		return berr
	})
	if err != nil {
		return true, err
	}

	taskState := task.GetTaskState()
	le.Debugf("task %q: %s", taskKey, taskState.String())

	if t.prevState != taskState && t.prevState != 0 {
		// re-scan job tasks to check if complete
		t.jt.objLoop.Wake()
	}
	t.prevState = taskState

	// assign us to any task which is not assigned
	taskPeerID := task.GetPeerId()
	if taskPeerID != "" && taskPeerID != t.jt.c.peerIDStr {
		// assigned to someone else
		return true, nil
	}
	if taskPeerID == "" {
		le.
			WithField("cluster-key", clusterKey).
			WithField("job-key", jobKey).
			WithField("task-key", taskKey).
			Debug("assigning task to cluster")
		_, _, err = forge_cluster.AssignTaskToCluster(ctx, ws, clusterKey, jobKey, taskKey, t.jt.c.peerID)
		if err != nil {
			return true, err
		}
	}

	// done
	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*taskTracker)(nil)).processState
