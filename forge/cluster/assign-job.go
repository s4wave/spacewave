package forge_cluster

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_job "github.com/s4wave/spacewave/forge/job"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewClusterAssignJobOp constructs a new ClusterAssignJobOp block.
func NewClusterAssignJobOp(clusterKey, jobKey string) *ClusterAssignJobOp {
	return &ClusterAssignJobOp{
		ClusterKey: clusterKey,
		JobKey:     jobKey,
	}
}

// AssignJobToCluster assigns an existing Job object to a Cluster.
// Returns seqno, sysErr, error.
func AssignJobToCluster(
	ctx context.Context,
	w world.WorldState,
	clusterKey, jobKey string,
	sender peer.ID,
) (uint64, bool, error) {
	op := NewClusterAssignJobOp(clusterKey, jobKey)
	return w.ApplyWorldOp(ctx, op, sender)
}

// Validate validates the operation cursorily.
// Should not block.
func (o *ClusterAssignJobOp) Validate() error {
	if o.GetClusterKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_key")
	}
	if o.GetJobKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "job_key")
	}
	return nil
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterAssignJobOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {

	// Read the cluster and job keys targeted by the operation.
	clusterKey, jobKey := o.GetClusterKey(), o.GetJobKey()

	// Check the job and cluster objects by decoding their bodies rather than
	// relying on the type index being immediately visible.
	cluster, err := LookupClusterBody(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}
	if err := cluster.Validate(); err != nil {
		return false, err
	}
	job, err := forge_job.LookupJobBody(ctx, worldHandle, jobKey)
	if err != nil {
		return false, err
	}
	if err := job.Validate(); err != nil {
		return false, err
	}

	// Link the job to the cluster.
	err = worldHandle.SetGraphQuad(ctx, NewClusterToJobQuad(clusterKey, jobKey))
	if err != nil {
		return false, err
	}

	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*ClusterAssignJobOp)(nil)
