package cluster_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/util/keyed"
	forge_cluster "github.com/aperturerobotics/forge/cluster"
	forge_job "github.com/aperturerobotics/forge/job"
	forge_task "github.com/aperturerobotics/forge/task"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/sirupsen/logrus"
)

// jobTracker tracks a Job managed by the Cluster.
type jobTracker struct {
	// c is the controller
	c *Controller
	// objKey is the job object key
	objKey string
	// objLoop is the object watcher loop
	objLoop *world_control.ObjectLoop
	// taskTrackers manages the list of task tracker routines.
	taskTrackers *keyed.Keyed[*taskTracker]
}

// newJobTracker constructs a new job tracker routine.
func (c *Controller) newJobTracker(key string) (keyed.Routine, *jobTracker) {
	tr := &jobTracker{
		c:      c,
		objKey: key,
	}
	tr.objLoop = world_control.NewObjectLoop(
		c.le.WithField("object-loop", "job-tracker"),
		key,
		tr.processState,
	)
	tr.taskTrackers = keyed.NewKeyed(tr.newTaskTracker)
	return tr.execute, tr
}

// execute executes the job tracker.
func (t *jobTracker) execute(ctx context.Context) error {
	objKey, le := t.objKey, t.c.le

	le.Debugf("starting job tracker: %s", objKey)
	t.taskTrackers.SetContext(ctx, true)
	return world_control.ExecuteBusObjectLoop(
		ctx,
		t.c.bus,
		t.c.conf.GetEngineId(),
		true,
		t.objLoop,
	)
}

// processState processes the state for the job.
func (t *jobTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	jobKey, clusterKey := t.objKey, t.c.objKey

	// check the <type> of the job object
	typesState := world_types.NewTypesState(ctx, ws)
	err = forge_job.CheckJobType(typesState, jobKey)
	if err != nil {
		return false, err
	}

	// unmarshal Job state
	var job *forge_job.Job
	_, err = world.AccessObject(ctx, ws.AccessWorldState, rootRef, func(bcs *block.Cursor) error {
		var berr error
		job, berr = forge_job.UnmarshalJob(bcs)
		if berr == nil {
			berr = job.Validate()
		}
		return berr
	})
	if err != nil {
		return true, err
	}

	// promote any pending jobs to running
	jobState := job.GetJobState()
	if jobState == forge_job.State_JobState_PENDING {
		le.Debugf("starting job: %s", jobKey)
		_, _, err = forge_cluster.StartJob(ctx, ws, clusterKey, jobKey, t.c.peerID)
		return true, err
	}

	// completed job
	if jobState != forge_job.State_JobState_RUNNING {
		return true, nil
	}

	// look up any Task associated with the job
	tasks, taskKeys, err := forge_job.CollectJobTasks(ctx, ws, jobKey)
	if err != nil {
		return true, err
	}

	// build list of non-complete Task
	var pendingTasks []string
	for i, task := range tasks {
		taskState := task.GetTaskState()
		if taskState != forge_task.State_TaskState_COMPLETE {
			pendingTasks = append(pendingTasks, taskKeys[i])
		}
	}

	// update the list of task watchers
	t.c.le.Debugf("found %d pending tasks: %v", len(pendingTasks), pendingTasks)
	t.taskTrackers.SyncKeys(pendingTasks, true)

	// if no tasks remain, promote to complete
	if len(pendingTasks) == 0 {
		t.c.le.Info("marking job as complete")
		_, _, err = forge_cluster.CompleteJob(ctx, ws, clusterKey, jobKey, t.c.peerID)
		if err != nil {
			return false, err
		}
	}

	// done
	return true, nil
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*jobTracker)(nil)).processState
