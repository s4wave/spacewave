package forge_cluster

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_job "github.com/s4wave/spacewave/forge/job"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// ClusterAssignJobOpId is the cluster assign job operation id.
var ClusterAssignJobOpId = ClusterTypeID + "/assign-job"

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

// Validate performs cursory validation of the operation.
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

// GetOperationTypeId returns the operation type identifier.
func (o *ClusterAssignJobOp) GetOperationTypeId() string {
	return ClusterAssignJobOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterAssignJobOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	clusterKey, jobKey := o.GetClusterKey(), o.GetJobKey()

	// check the <type> of the job and cluster objects
	err = CheckClusterType(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}

	err = forge_job.CheckJobType(ctx, worldHandle, jobKey)
	if err != nil {
		return false, err
	}

	// assign the job to the cluster
	err = worldHandle.SetGraphQuad(ctx, NewClusterToJobQuad(clusterKey, jobKey))
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ClusterAssignJobOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *ClusterAssignJobOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *ClusterAssignJobOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ClusterAssignJobOp)(nil))
