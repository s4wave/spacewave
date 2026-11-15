package forge_cluster

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	forge_job "github.com/s4wave/spacewave/forge/job"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

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

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterStartJobOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {

	// Read the cluster and job keys targeted by the operation.
	clusterKey, jobKey := o.GetClusterKey(), o.GetJobKey()

	// check the <type> of the cluster and job objects
	if err := CheckClusterType(ctx, worldHandle, clusterKey); err != nil {
		return false, err
	}

	// Confirm the job object type.
	if err := forge_job.CheckJobType(ctx, worldHandle, jobKey); err != nil {
		return false, err
	}

	// check if the job is assigned to the cluster
	hasJob, err := CheckClusterHasJob(ctx, worldHandle, clusterKey, jobKey)
	if err != nil {
		return false, err
	}
	if !hasJob {
		return false, world.NewOpRefusedError(errors.Errorf("cluster %s not linked to job %s", clusterKey, jobKey))
	}

	// transition job to running
	_, _, err = world.AccessWorldObject(ctx, worldHandle, jobKey, true, func(bcs *block.Cursor) error {

		// Decode and validate the job body.
		job, err := forge_job.UnmarshalJob(ctx, bcs)
		if err == nil {
			err = job.Validate()
		}
		if err != nil {
			return err
		}

		// Transition the job to running and clear its result.
		job.JobState = forge_job.State_JobState_RUNNING
		job.Result = nil
		if err := job.Validate(); err != nil {
			return err
		}

		// Store the updated job block.
		bcs.SetBlock(job, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*ClusterStartJobOp)(nil)
