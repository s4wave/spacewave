package forge_cluster

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
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
	return w.ApplyWorldOp(ctx, op, sender)
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

	// Confirm the cluster, job, and task object types.
	err = CheckClusterType(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}

	err = forge_job.CheckJobType(ctx, worldHandle, jobKey)
	if err != nil {
		return false, err
	}

	err = forge_task.CheckTaskType(ctx, worldHandle, taskKey)
	if err != nil {
		return false, err
	}

	// Load the cluster record and its controlling peer.
	cluster, _, err := LookupCluster(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}
	clusterPeerID, err := cluster.ParsePeerID()
	if err != nil {
		return false, err
	}
	clusterPeerIDStr := clusterPeerID.String()
	if clusterPeerIDStr == "" {
		return false, errors.Wrap(peer.ErrEmptyPeerID, "cluster")
	}

	// Require the sender to match the cluster's controlling peer.
	senderPeerIDStr := sender.String()
	if senderPeerIDStr != clusterPeerIDStr {
		return false, errors.Errorf("tx sender %s does not match cluster %s", senderPeerIDStr, clusterPeerIDStr)
	}

	// Confirm the job is linked to the cluster.
	err = EnsureClusterHasJob(ctx, worldHandle, clusterKey, jobKey)
	if err != nil {
		return false, err
	}

	// Load the job record.
	job, _, err := forge_job.LookupJob(ctx, worldHandle, jobKey)
	if err != nil {
		return false, err
	}

	// Require the job to be running before assigning the task.
	err = job.GetJobState().EnsureMatches(forge_job.State_JobState_RUNNING)
	if err != nil {
		return false, err
	}

	// Confirm the task is linked to the job.
	err = forge_job.EnsureJobHasTask(ctx, worldHandle, jobKey, taskKey)
	if err != nil {
		return false, err
	}

	// Assign the task to the cluster when it is unclaimed.
	_, _, err = world.AccessWorldObject(ctx, worldHandle, taskKey, true, func(bcs *block.Cursor) error {
		task, err := forge_task.UnmarshalTask(ctx, bcs)
		if err != nil {
			return err
		}

		// Reject tasks that already have a peer assignment.
		taskPeerID := task.GetPeerId()
		if taskPeerID != "" {
			return errors.Errorf("task already assigned to %s", taskPeerID)
		}

		// Store the cluster peer assignment.
		task.PeerId = clusterPeerIDStr
		bcs.SetBlock(task, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	// Link the cluster keypair to the assigned task.
	_, _, err = identity_world.LinkObjectToKeypair(ctx, worldHandle, sender, taskKey, clusterPeerID, "", nil)
	if err != nil {
		return false, err
	}

	// Finish after the task assignment is linked.
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
var _ world.Operation = (*ClusterAssignTaskOp)(nil)
