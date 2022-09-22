package forge_cluster

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	forge_worker "github.com/aperturerobotics/forge/worker"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ClusterAssignWorkerOpId is the cluster assign job operation id.
var ClusterAssignWorkerOpId = ClusterTypeID + "/assign-worker"

// NewClusterAssignWorkerOp constructs a new ClusterAssignWorkerOp block.
func NewClusterAssignWorkerOp(clusterKey, workerKey string) *ClusterAssignWorkerOp {
	return &ClusterAssignWorkerOp{
		ClusterKey: clusterKey,
		WorkerKey:  workerKey,
	}
}

// AssignWorkerToCluster assigns an existing Worker object to a Cluster.
// Returns seqno, sysErr, error.
func AssignWorkerToCluster(
	ctx context.Context,
	w world.WorldState,
	clusterKey, workerKey string,
	sender peer.ID,
) (uint64, bool, error) {
	op := NewClusterAssignWorkerOp(clusterKey, workerKey)
	return w.ApplyWorldOp(op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *ClusterAssignWorkerOp) Validate() error {
	if o.GetClusterKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_key")
	}
	if o.GetWorkerKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "worker_key")
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *ClusterAssignWorkerOp) GetOperationTypeId() string {
	return ClusterAssignWorkerOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterAssignWorkerOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	clusterKey, workerKey := o.GetClusterKey(), o.GetWorkerKey()

	// check the <type> of the worker and cluster objects
	typesState := world_types.NewTypesState(ctx, worldHandle)

	err = CheckClusterType(typesState, clusterKey)
	if err != nil {
		return false, err
	}

	err = forge_worker.CheckWorkerType(typesState, workerKey)
	if err != nil {
		return false, err
	}

	// assign the worker to the cluster
	err = worldHandle.SetGraphQuad(NewClusterToWorkerQuad(clusterKey, workerKey))
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ClusterAssignWorkerOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *ClusterAssignWorkerOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *ClusterAssignWorkerOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ClusterAssignWorkerOp)(nil))
