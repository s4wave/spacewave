package forge_cluster

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewClusterCreateOp constructs a new ClusterCreateOp block.
func NewClusterCreateOp(clusterKey, name string, peerID peer.ID) *ClusterCreateOp {
	return &ClusterCreateOp{
		ClusterKey: clusterKey,
		Name:       name,
		PeerId:     peerID.String(),
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

// Validate validates the operation cursorily.
// PeerId is optional here: defaults to sender in ApplyWorldOp.
func (o *ClusterCreateOp) Validate() error {
	if o.GetClusterKey() == "" {
		return world.ErrEmptyObjectKey
	}
	if err := identity.ValidateEntityID(o.GetName()); err != nil {
		return err
	}
	return nil
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

	// Default peer_id to sender when not specified.
	if o.GetPeerId() == "" && sender != "" {
		o.PeerId = sender.String()
	}

	// Build and validate the Cluster body from the operation.
	clusterKey := o.GetClusterKey()
	clstr := o.BuildCluster()
	err = clstr.Validate()
	if err != nil {
		return false, err
	}

	// Create the Cluster world object and release its handle.
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, worldHandle, clusterKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(clstr, true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
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

// _ is a type assertion
var _ world.Operation = (*ClusterCreateOp)(nil)
