package forge_job

import (
	"context"

	"github.com/aperturerobotics/cayley"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_control "github.com/s4wave/spacewave/db/world/control"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// LookupJob looks up a Job in the world.
func LookupJob(ctx context.Context, ws world.WorldState, objKey string) (*Job, world.ObjectState, error) {
	return world.LookupObject[*Job](ctx, ws, objKey, NewJobBlock)
}

// CheckJobType checks the type graph quad for a cluster.
func CheckJobType(ctx context.Context, ws world.WorldState, objKey string) error {
	return world_types.CheckObjectType(ctx, ws, objKey, JobTypeID)
}

// CheckJobHasTask checks if the job is linked to a task.
func CheckJobHasTask(ctx context.Context, w world.WorldState, jobKey, taskKey string) (bool, error) {
	gq, err := w.LookupGraphQuads(ctx, world.NewGraphQuad(
		world.KeyToGraphValue(jobKey).String(),
		PredJobToTask.String(),
		world.KeyToGraphValue(taskKey).String(),
		"",
	), 1)
	if err != nil {
		return false, err
	}
	return len(gq) != 0, nil
}

// EnsureJobHasTask checks if the job has the task and returns an error otherwise.
func EnsureJobHasTask(ctx context.Context, w world.WorldState, jobKey, taskKey string) error {
	hasTask, err := CheckJobHasTask(ctx, w, jobKey, taskKey)
	if err == nil && !hasTask {
		err = errors.Errorf("job %s does not have task %s", jobKey, taskKey)
	}
	return err
}

// CreateJobWithTasks creates a pending Job object in the world.
//
// TasksPeer sets the peer ID to set on the tasks. Can be empty.
func CreateJobWithTasks(
	ctx context.Context,
	ws world.WorldState,
	sender peer.ID,
	objKey string,
	tasks map[string]*forge_target.Target,
	tasksPeer peer.ID,
	ts *timestamp.Timestamp,
) (world.ObjectState, *bucket.ObjectRef, error) {
	njob := &Job{
		JobState:  State_JobState_PENDING,
		Timestamp: ts,
	}
	if err := njob.Validate(); err != nil {
		return nil, nil, err
	}
	objState, rootRef, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(njob, true)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// create the <type> ref
	err = world_types.SetObjectType(ctx, ws, objKey, JobTypeID)
	if err != nil {
		return objState, rootRef, err
	}

	// create the tasks & targets & links
	for taskName, taskTgt := range tasks {
		if err := forge_task.ValidateName(taskName); err != nil {
			return nil, nil, errors.Wrapf(err, "tasks[%s]", taskName)
		}
		taskKey := NewJobTaskKey(objKey, taskName)
		replicas := uint32(1)
		_, _, err = forge_task.CreateTaskWithTarget(ctx, ws, sender, taskKey, taskName, taskTgt, tasksPeer, replicas, ts)
		if err != nil {
			return objState, rootRef, errors.Wrapf(err, "tasks[%s]", taskName)
		}

		// create parent link
		err = world_parent.SetObjectParent(ctx, ws, taskKey, objKey, false)
		if err != nil {
			return objState, rootRef, err
		}

		// create job -> task link
		err = ws.SetGraphQuad(ctx, NewJobToTaskQuad(objKey, taskKey))
		if err != nil {
			return objState, rootRef, err
		}
	}

	return objState, rootRef, nil
}

// WaitJobComplete waits until the Job is in the COMPLETE state.
func WaitJobComplete(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	jobObjectKey string,
) (*Job, error) {
	// wait for Job to complete
	var finalState *Job
	var lastState State
	loop := world_control.NewWatchLoop(
		le,
		jobObjectKey,
		world_control.NewWaitForStateHandler(
			func(ctx context.Context, ws world.WorldState, obj world.ObjectState, rootCs *block.Cursor, rev uint64) (bool, error) {
				if obj == nil {
					return true, nil
				}
				job, err := UnmarshalJob(ctx, rootCs)
				if err != nil {
					return true, err
				}
				nextState := job.GetJobState()
				if nextState != lastState {
					lastState = nextState
					le.Debugf("job is in state: %s", nextState.String())
					if ferr := job.GetResult().GetFailError(); ferr != "" {
						le.WithError(errors.New(ferr)).Warn("job failed")
					}
				}
				complete := job.IsComplete()
				if complete {
					finalState = job
				}
				return !complete, nil
			},
		),
	)
	if err := loop.Execute(ctx, ws); err != nil {
		return nil, err
	}
	return finalState, nil
}

// ListJobTasks lists all Execution object keys that are linked to by the Job.
func ListJobTasks(ctx context.Context, w world.WorldState, jobKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		w,
		jobKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return p.Out(PredJobToTask), nil
		},
	)
}

// CollectJobTasks collects all Executions linked to by the Job.
// If any of the linked tasks are invalid, returns an error.
func CollectJobTasks(
	ctx context.Context,
	ws world.WorldState,
	jobObjectKeys ...string,
) ([]*forge_task.Task, []string, error) {
	kpObjectKeys, err := ListJobTasks(ctx, ws, jobObjectKeys...)
	if err != nil {
		return nil, nil, err
	}

	tasks := make([]*forge_task.Task, len(kpObjectKeys))
	for i, objKey := range kpObjectKeys {
		tasks[i], _, err = forge_task.LookupTask(ctx, ws, objKey)
		if err != nil {
			return nil, nil, err
		}
	}

	return tasks, kpObjectKeys, nil
}
