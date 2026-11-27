package forge_task_ops

import (
	"context"

	"github.com/pkg/errors"
	space_exec_noop "github.com/s4wave/spacewave/core/forge/exec/noop"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Validate cursorily validates the operation.
func (o *ForgeTaskCreateOp) Validate() error {
	if o.GetTaskKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "task_key")
	}
	if err := forge_task.ValidateName(o.GetName()); err != nil {
		return err
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ForgeTaskCreateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	// Prepare the task and optional job identities for placement and linking.
	taskKey := o.GetTaskKey()
	jobKey := o.GetJobKey()
	var placement *forge_worker.Placement

	// If a job key is provided, verify the job exists and can be decoded
	// as a Job.
	if jobKey != "" {
		job, err := forge_job.LookupJobBody(ctx, ws, jobKey)
		if err != nil {
			return false, errors.Wrap(err, "job")
		}
		if err := job.Validate(); err != nil {
			return false, errors.Wrap(err, "job")
		}
		placement = job.GetPlacement()
	}

	// Create the task with the default noop exec target.
	tgt := space_exec_noop.NewTarget()
	var createdObject world.ObjectState
	createdObject, _, err = forge_task.CreateTaskWithTarget(ctx, ws, sender, taskKey, o.GetName(), tgt, "", 1, placement, o.GetTimestamp())
	world.ReleaseObjectState(createdObject)
	if err != nil {
		return false, err
	}

	// Link the task to the job if provided.
	if jobKey != "" {
		if err := world_parent.SetObjectParent(ctx, ws, taskKey, jobKey, false); err != nil {
			return false, err
		}
		if err := ws.SetGraphQuad(ctx, forge_job.NewJobToTaskQuad(jobKey, taskKey)); err != nil {
			return false, err
		}
	}

	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*ForgeTaskCreateOp)(nil)
