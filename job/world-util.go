package forge_job

import (
	"context"

	forge_task "github.com/aperturerobotics/forge/task"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/cayleygraph/cayley"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// LookupJob looks up a Job in the world.
func LookupJob(ctx context.Context, ws world.WorldState, objKey string) (*Job, error) {
	return world.LookupObject[*Job](ctx, ws, objKey, NewJobBlock)
}

// CheckJobType checks the type graph quad for a cluster.
func CheckJobType(typesState *world_types.TypesState, objKey string) error {
	jobType, err := typesState.GetObjectType(objKey)
	if err != nil {
		return err
	}
	if jobType != JobTypeID {
		return errors.Errorf("expected job type %s but got %q", JobTypeID, jobType)
	}
	return err
}

// CheckJobHasTask checks if the job is linked to a task.
func CheckJobHasTask(ctx context.Context, w world.WorldState, jobKey, taskKey string) (bool, error) {
	gq, err := w.LookupGraphQuads(world.NewGraphQuad(
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
	loop := world_control.NewObjectLoop(
		le,
		jobObjectKey,
		world_control.NewWaitForStateHandler(
			func(obj world.ObjectState, rootCs *block.Cursor, rev uint64) (bool, error) {
				if obj == nil {
					return true, nil
				}
				job, err := UnmarshalJob(rootCs)
				if err != nil {
					return false, err
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
		tasks[i], err = forge_task.LookupTask(ctx, ws, objKey)
		if err != nil {
			return nil, nil, err
		}
	}

	return tasks, kpObjectKeys, nil
}
