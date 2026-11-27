package forge_cluster

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

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
	return w.ApplyWorldOp(ctx, op, sender)
}

// Validate validates the operation cursorily.
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

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterAssignWorkerOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {

	// Read the cluster and worker keys targeted by the operation.
	clusterKey, workerKey := o.GetClusterKey(), o.GetWorkerKey()

	// Confirm the cluster object type.
	err = CheckClusterType(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}

	// Confirm the worker object type.
	err = forge_worker.CheckWorkerType(ctx, worldHandle, workerKey)
	if err != nil {
		return false, err
	}

	// Link the worker to the cluster.
	err = worldHandle.SetGraphQuad(ctx, NewClusterToWorkerQuad(clusterKey, workerKey))
	if err != nil {
		return false, err
	}

	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*ClusterAssignWorkerOp)(nil)
