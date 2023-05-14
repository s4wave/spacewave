package forge_cluster

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	identity_world "github.com/aperturerobotics/identity/world"
	"github.com/sirupsen/logrus"
)

// ClusterCreateOpId is the Cluster create operation id.
var ClusterCreateOpId = ClusterTypeID + "/create"

// NewClusterCreateOp constructs a new ClusterCreateOp block.
func NewClusterCreateOp(clusterKey, name string, peerID peer.ID) *ClusterCreateOp {
	return &ClusterCreateOp{
		ClusterKey: clusterKey,
		Name:       name,
		PeerId:     peerID.Pretty(),
	}
}

// CreateCluster stores a Cluster in a object associated with an existing Cluster.
// Optionally creates keypairs linked to the Cluster.
// Returns seqno, sysErr, error.
func CreateCluster(
	ctx context.Context,
	w world.WorldState,
	clusterKey string,
	name string,
	clusterPeerID peer.ID,
	sender peer.ID,
) (uint64, bool, error) {
	op := NewClusterCreateOp(clusterKey, name, clusterPeerID)
	return w.ApplyWorldOp(ctx, op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *ClusterCreateOp) Validate() error {
	if o.GetClusterKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := o.BuildCluster().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *ClusterCreateOp) GetOperationTypeId() string {
	return ClusterCreateOpId
}

// BuildCluster builds the Cluster object from the create op.
func (o *ClusterCreateOp) BuildCluster() *Cluster {
	return &Cluster{
		Name:   o.GetName(),
		PeerId: o.GetPeerId(),
	}
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterCreateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	clusterKey := o.GetClusterKey()
	clstr := o.BuildCluster()
	err = clstr.Validate()
	if err != nil {
		return false, err
	}

	_, _, err = world.CreateWorldObject(ctx, worldHandle, clusterKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(clstr, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	// create the <type> ref
	err = world_types.SetObjectType(ctx, worldHandle, clusterKey, ClusterTypeID)
	if err != nil {
		return false, err
	}

	// create the keypair and link to it if necessary
	peerID, err := clstr.ParsePeerID()
	if err != nil {
		return false, err
	}
	_, _, err = identity_world.LinkObjectToKeypair(ctx, worldHandle, sender, clusterKey, peerID, "", nil)
	if err != nil {
		return false, err
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ClusterCreateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *ClusterCreateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *ClusterCreateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ClusterCreateOp)(nil))
