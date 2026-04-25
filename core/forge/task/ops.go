package forge_task_ops

import (
	"context"

	"github.com/pkg/errors"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ForgeTaskCreateOpId is the operation id for ForgeTaskCreateOp.
var ForgeTaskCreateOpId = "spacewave/forge/task/create"

// GetOperationTypeId returns the operation type identifier.
func (o *ForgeTaskCreateOp) GetOperationTypeId() string {
	return ForgeTaskCreateOpId
}

// Validate performs cursory validation of the operation.
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
	taskKey := o.GetTaskKey()
	jobKey := o.GetJobKey()

	// If a job key is provided, verify it exists and can be decoded as a Job.
	if jobKey != "" {
		job, _, err := forge_job.LookupJob(ctx, ws, jobKey)
		if err != nil {
			return false, errors.Wrap(err, "job")
		}
		if err := job.Validate(); err != nil {
			return false, errors.Wrap(err, "job")
		}
	}

	// Create the task with the default noop exec target.
	tgt := space_exec.NewNoopTarget()
	_, _, err = forge_task.CreateTaskWithTarget(ctx, ws, sender, taskKey, o.GetName(), tgt, "", 1, o.GetTimestamp())
	if err != nil {
		return false, err
	}

	// Link to job if provided.
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

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ForgeTaskCreateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *ForgeTaskCreateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *ForgeTaskCreateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupForgeTaskCreateOp looks up a ForgeTaskCreateOp operation type.
func LookupForgeTaskCreateOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == ForgeTaskCreateOpId {
		return &ForgeTaskCreateOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*ForgeTaskCreateOp)(nil))
