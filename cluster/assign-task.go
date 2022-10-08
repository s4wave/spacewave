package forge_cluster

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_job "github.com/aperturerobotics/forge/job"
	forge_task "github.com/aperturerobotics/forge/task"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	identity_world "github.com/aperturerobotics/identity/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ClusterAssignTaskOpId is the cluster assign task operation id.
var ClusterAssignTaskOpId = ClusterTypeID + "/assign-task"

// NewClusterAssignTaskOp constructs a new ClusterAssignTaskOp block.
func NewClusterAssignTaskOp(clusterKey, jobKey, taskKey string) *ClusterAssignTaskOp {
	return &ClusterAssignTaskOp{
		ClusterKey: clusterKey,
		JobKey:     jobKey,
		TaskKey:    taskKey,
	}
}

// AssignTaskToCluster assigns an existing Task object to a Cluster.
// Returns seqno, sysErr, error.
func AssignTaskToCluster(
	ctx context.Context,
	w world.WorldState,
	clusterKey, jobKey, taskKey string,
	sender peer.ID,
) (uint64, bool, error) {
	op := NewClusterAssignTaskOp(clusterKey, jobKey, taskKey)
	return w.ApplyWorldOp(op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *ClusterAssignTaskOp) Validate() error {
	if o.GetClusterKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_key")
	}
	if o.GetJobKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "job_key")
	}
	if o.GetTaskKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "task_key")
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *ClusterAssignTaskOp) GetOperationTypeId() string {
	return ClusterAssignTaskOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterAssignTaskOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	clusterKey, jobKey, taskKey := o.GetClusterKey(), o.GetJobKey(), o.GetTaskKey()

	// check the <type> of the job, cluster, and task objects
	typesState := world_types.NewTypesState(ctx, worldHandle)

	err = CheckClusterType(typesState, clusterKey)
	if err != nil {
		return false, err
	}

	err = forge_job.CheckJobType(typesState, jobKey)
	if err != nil {
		return false, err
	}

	err = forge_task.CheckTaskType(typesState, taskKey)
	if err != nil {
		return false, err
	}

	// unmarshal the cluster
	cluster, err := LookupCluster(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}
	clusterPeerID, err := cluster.ParsePeerID()
	if err != nil {
		return false, err
	}
	clusterPeerIDStr := clusterPeerID.Pretty()
	if clusterPeerIDStr == "" {
		return false, errors.Wrap(peer.ErrEmptyPeerID, "cluster")
	}

	// ensure the sender matches the cluster peer id
	senderPeerIDStr := sender.Pretty()
	if senderPeerIDStr != clusterPeerIDStr {
		return false, errors.Errorf("tx sender %s does not match cluster %s", senderPeerIDStr, clusterPeerIDStr)
	}

	// ensure the cluster and job are linked
	err = EnsureClusterHasJob(ctx, worldHandle, clusterKey, jobKey)
	if err != nil {
		return false, err
	}

	// unmarshal the job
	job, err := forge_job.LookupJob(ctx, worldHandle, jobKey)
	if err != nil {
		return false, err
	}

	// ensure the job is running
	err = job.GetJobState().EnsureMatches(forge_job.State_JobState_RUNNING)
	if err != nil {
		return false, err
	}

	// ensure the job and task are linked
	err = forge_job.EnsureJobHasTask(ctx, worldHandle, jobKey, taskKey)
	if err != nil {
		return false, err
	}

	// update the task
	_, _, err = world.AccessWorldObject(ctx, worldHandle, taskKey, true, func(bcs *block.Cursor) error {
		task, err := forge_task.UnmarshalTask(bcs)
		if err != nil {
			return err
		}

		// check if the task is assigned already
		taskPeerID := task.GetPeerId()
		if taskPeerID != "" {
			return errors.Errorf("task already assigned to %s", taskPeerID)
		}

		// assign the task to the cluster
		task.PeerId = clusterPeerIDStr
		bcs.SetBlock(task, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	// create the keypair and link to it if necessary
	_, _, err = identity_world.LinkObjectToKeypair(ctx, worldHandle, sender, taskKey, clusterPeerID, "", nil)
	if err != nil {
		return false, err
	}

	// done
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ClusterAssignTaskOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *ClusterAssignTaskOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *ClusterAssignTaskOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ClusterAssignTaskOp)(nil))
