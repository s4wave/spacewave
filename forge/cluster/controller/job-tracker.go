package cluster_controller

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
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
	// c is the controller
	c *Controller
	// objKey is the job object key
	objKey string
	// objLoop is the object watcher loop
	objLoop *world_control.WatchLoop
	// taskTrackers manages the list of task tracker routines.
	taskTrackers *keyed.Keyed[string, *taskTracker]
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
		tr.processState,
	)
	tr.taskTrackers = keyed.NewKeyedWithLogger(tr.newTaskTracker, c.le)
	return tr.execute, tr
}

// execute executes the job tracker.
func (jt *jobTracker) execute(ctx context.Context) error {
	objKey, le := jt.objKey, jt.c.le

	le.Debugf("starting job tracker: %s", objKey)
	jt.taskTrackers.SetContext(ctx, true)
	return world_control.ExecuteBusWatchLoop(
		ctx,
		jt.c.bus,
		jt.c.conf.GetEngineId(),
		true,
		jt.objLoop,
	)
}

// processState processes the state for the job.
func (jt *jobTracker) processState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	jobKey, clusterKey := jt.objKey, jt.c.objKey

	// check the <type> of the job object
	err = forge_job.CheckJobType(ctx, ws, jobKey)
	if err != nil {
		return false, err
	}

	// unmarshal Job state
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

	// promote any pending jobs to running
	jobState := job.GetJobState()
	if jobState == forge_job.State_JobState_PENDING {
		le.Debugf("starting job: %s", jobKey)
		_, _, err = forge_cluster.StartJob(ctx, ws, clusterKey, jobKey, jt.c.peerID)
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
	jt.c.le.Debugf("found %d pending tasks: %v", len(pendingTasks), pendingTasks)
	jt.taskTrackers.SyncKeys(pendingTasks, true)

	// if no tasks remain, promote to complete
	if len(pendingTasks) == 0 {
		jt.c.le.Info("marking job as complete")
		_, _, err = forge_cluster.CompleteJob(ctx, ws, clusterKey, jobKey, jt.c.peerID)
		if err != nil {
			return false, err
		}
	}

	// done
	return true, nil
}

// _ is a type assertion
var _ world_control.WatchLoopHandler = ((*jobTracker)(nil)).processState
