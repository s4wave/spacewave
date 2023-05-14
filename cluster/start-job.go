package forge_cluster

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_job "github.com/aperturerobotics/forge/job"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ClusterStartJobOpId is the cluster start job operation id.
var ClusterStartJobOpId = ClusterTypeID + "/start-job"

// NewClusterStartJobOp constructs a new ClusterStartJobOp block.
func NewClusterStartJobOp(clusterKey, jobKey string) *ClusterStartJobOp {
	return &ClusterStartJobOp{
		ClusterKey: clusterKey,
		JobKey:     jobKey,
	}
}

// StartJob starts an existing Job linked to the Cluster.
// Returns seqno, sysErr, error.
func StartJob(
	ctx context.Context,
	w world.WorldState,
	clusterKey, jobKey string,
	sender peer.ID,
) (uint64, bool, error) {
	op := NewClusterStartJobOp(clusterKey, jobKey)
	return w.ApplyWorldOp(ctx, op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *ClusterStartJobOp) Validate() error {
	if o.GetClusterKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_key")
	}
	if o.GetJobKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "job_key")
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *ClusterStartJobOp) GetOperationTypeId() string {
	return ClusterStartJobOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterStartJobOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	clusterKey, jobKey := o.GetClusterKey(), o.GetJobKey()

	// check the <type> of the cluster and job objects
	if err := CheckClusterType(ctx, worldHandle, clusterKey); err != nil {
		return false, err
	}

	if err := forge_job.CheckJobType(ctx, worldHandle, jobKey); err != nil {
		return false, err
	}

	// check if the job is assigned to the cluster
	hasJob, err := CheckClusterHasJob(ctx, worldHandle, clusterKey, jobKey)
	if err != nil {
		return false, err
	}
	if !hasJob {
		return false, errors.Errorf("cluster %s not linked to job %s", clusterKey, jobKey)
	}

	// transition job to running
	_, _, err = world.AccessWorldObject(ctx, worldHandle, jobKey, true, func(bcs *block.Cursor) error {
		job, err := forge_job.UnmarshalJob(ctx, bcs)
		if err == nil {
			err = job.Validate()
		}
		if err != nil {
			return err
		}

		job.JobState = forge_job.State_JobState_RUNNING
		if err := job.Validate(); err != nil {
			return err
		}

		bcs.SetBlock(job, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ClusterStartJobOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *ClusterStartJobOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *ClusterStartJobOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ClusterStartJobOp)(nil))
