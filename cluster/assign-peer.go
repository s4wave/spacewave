package forge_cluster

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	identity_world "github.com/aperturerobotics/identity/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ClusterAssignPeerOpId is the cluster assign peer operation id.
var ClusterAssignPeerOpId = ClusterTypeID + "/assign-peer"

// NewClusterAssignPeerOp constructs a new ClusterAssignPeerOp block.
func NewClusterAssignPeerOp(clusterKey string, peerID peer.ID) *ClusterAssignPeerOp {
	return &ClusterAssignPeerOp{
		ClusterKey: clusterKey,
		PeerId:     peerID.Pretty(),
	}
}

// AssignClusterLeaderPeer assigns the Cluster controller peer.
// Returns seqno, sysErr, error.
func AssignClusterLeaderPeer(
	ctx context.Context,
	w world.WorldState,
	sender peer.ID,
	clusterKey string, leaderPeer peer.ID,
) (uint64, bool, error) {
	op := NewClusterAssignPeerOp(clusterKey, leaderPeer)
	return w.ApplyWorldOp(op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *ClusterAssignPeerOp) Validate() error {
	if o.GetClusterKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_key")
	}
	if o.GetPeerId() == "" {
		return peer.ErrPeerIDEmpty
	}
	if _, err := o.ParsePeerID(); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (o *ClusterAssignPeerOp) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(o.GetPeerId())
}

// GetOperationTypeId returns the operation type identifier.
func (o *ClusterAssignPeerOp) GetOperationTypeId() string {
	return ClusterAssignPeerOpId
}

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterAssignPeerOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	clusterKey := o.GetClusterKey()
	peerID, err := o.ParsePeerID()
	if err != nil {
		return false, err
	}

	// if the peer id matches the current, return nil
	peerIDStr := peerID.Pretty()
	if o.GetPeerId() == peerIDStr {
		return false, nil
	}

	// check the <type> of the cluster
	typesState := world_types.NewTypesState(ctx, worldHandle)
	err = CheckClusterType(typesState, clusterKey)
	if err != nil {
		return false, err
	}

	var cluster *Cluster
	_, _, err = world.AccessWorldObject(ctx, worldHandle, clusterKey, true, func(bcs *block.Cursor) error {
		var err error
		cluster, err = UnmarshalCluster(bcs)
		if err == nil {
			err = cluster.Validate()
		}
		if err != nil {
			return err
		}

		clusterPeerID, err := cluster.ParsePeerID()
		if err != nil {
			return err
		}
		clusterPeerIDStr := clusterPeerID.Pretty()
		if clusterPeerIDStr == "" {
			return errors.Wrap(peer.ErrPeerIDEmpty, "cluster")
		}

		// ensure the sender matches the cluster peer id
		senderPeerIDStr := sender.Pretty()
		if senderPeerIDStr != clusterPeerIDStr {
			return errors.Errorf("tx sender %s does not match cluster %s", senderPeerIDStr, clusterPeerIDStr)
		}

		// update the cluster peer id field
		cluster.PeerId = peerIDStr
		bcs.SetBlock(cluster, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	// clear any old keypair links
	oldKpKeys, err := identity_world.ListObjectKeypairs(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}
	for _, oldKpKey := range oldKpKeys {
		err = worldHandle.DeleteGraphQuad(world.NewGraphQuadWithKeys(
			clusterKey,
			identity_world.PredObjectToKeypair.String(),
			oldKpKey,
			"",
		))
		if err != nil {
			return false, err
		}
	}

	// create the keypair and link to it if necessary
	_, _, err = identity_world.LinkObjectToKeypair(ctx, worldHandle, sender, clusterKey, peerID, "", nil)
	if err != nil {
		return false, err
	}

	// done
	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *ClusterAssignPeerOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *ClusterAssignPeerOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *ClusterAssignPeerOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*ClusterAssignPeerOp)(nil))
