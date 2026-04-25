package forge_job_ops

import (
	"context"

	"github.com/pkg/errors"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ForgeJobCreateOpId is the operation id for ForgeJobCreateOp.
var ForgeJobCreateOpId = "spacewave/forge/job/create"

// GetOperationTypeId returns the operation type identifier.
func (o *ForgeJobCreateOp) GetOperationTypeId() string {
	return ForgeJobCreateOpId
}

// Validate performs cursory validation of the operation.
func (o *ForgeJobCreateOp) Validate() error {
	if o.GetJobKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "job_key")
	}
	if o.GetClusterKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_key")
	}
	if len(o.GetTaskDefs()) == 0 {
		return errors.New("at least one task definition is required")
	}
	for i, td := range o.GetTaskDefs() {
		if err := forge_task.ValidateName(td.GetName()); err != nil {
			return errors.Wrapf(err, "task_defs[%d]", i)
		}
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ForgeJobCreateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	clusterKey := o.GetClusterKey()
	jobKey := o.GetJobKey()

	// Check the cluster exists and can be decoded as a Cluster object.
	cluster, _, err := forge_cluster.LookupCluster(ctx, ws, clusterKey)
	if err != nil {
		return false, errors.Wrap(err, "cluster")
	}
	if err := cluster.Validate(); err != nil {
		return false, errors.Wrap(err, "cluster")
	}

	// Build the tasks map from the task definitions.
	tasks := make(map[string]*forge_target.Target, len(o.GetTaskDefs()))
	for _, td := range o.GetTaskDefs() {
		tasks[td.GetName()] = space_exec.NewNoopTarget()
	}

	// Create the job with tasks.
	_, _, err = forge_job.CreateJobWithTasks(ctx, ws, sender, jobKey, tasks, "", o.GetTimestamp())
	if err != nil {
		return false, err
	}

	// Assign the job to the cluster.
	assignOp := forge_cluster.NewClusterAssignJobOp(clusterKey, jobKey)
	_, _, err = ws.ApplyWorldOp(ctx, assignOp, sender)
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ForgeJobCreateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
func (o *ForgeJobCreateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *ForgeJobCreateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupForgeJobCreateOp looks up a ForgeJobCreateOp operation type.
func LookupForgeJobCreateOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == ForgeJobCreateOpId {
		return &ForgeJobCreateOp{}, nil
	}
	return nil, nil
}

// _ is a type assertion
var _ world.Operation = ((*ForgeJobCreateOp)(nil))
