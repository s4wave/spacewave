package forge_cluster

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
	"github.com/sirupsen/logrus"
)

// NewClusterAssignPeerOp constructs a new ClusterAssignPeerOp block.
func NewClusterAssignPeerOp(clusterKey string, peerID peer.ID) *ClusterAssignPeerOp {
	return &ClusterAssignPeerOp{
		ClusterKey: clusterKey,
		PeerId:     peerID.String(),
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
	return w.ApplyWorldOp(ctx, op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *ClusterAssignPeerOp) Validate() error {
	if o.GetClusterKey() == "" {
		return errors.Wrap(world.ErrEmptyObjectKey, "cluster_key")
	}
	if o.GetPeerId() == "" {
		return peer.ErrEmptyPeerID
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

// ApplyWorldOp applies the operation as a world operation.
func (o *ClusterAssignPeerOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {

	// Parse the operation's cluster key and replacement peer ID.
	clusterKey := o.GetClusterKey()
	peerID, err := o.ParsePeerID()
	if err != nil {
		return false, err
	}

	// Format the replacement peer ID as a string for comparison.
	peerIDStr := peerID.String()

	// Confirm the cluster object type.
	err = CheckClusterType(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}

	// Update the cluster object inside a read-write transaction.
	var changed bool
	_, _, err = world.AccessWorldObject(ctx, worldHandle, clusterKey, true, func(bcs *block.Cursor) error {

		// Decode the cluster, check authorization, and store the new peer ID.
		var err error
		cluster, err := UnmarshalCluster(ctx, bcs)
		if err == nil {
			err = cluster.Validate()
		}
		if err != nil {
			return err
		}

		// Skip the update when the cluster already runs this peer.
		clusterPeerID, err := cluster.ParsePeerID()
		if err != nil {
			return err
		}
		clusterPeerIDStr := clusterPeerID.String()
		if clusterPeerIDStr == "" {
			return world.NewOpRefusedError(errors.Wrap(peer.ErrEmptyPeerID, "cluster"))
		}
		if clusterPeerIDStr == peerIDStr {
			return nil
		}

		// Require the sender to match the cluster's current peer.
		senderPeerIDStr := sender.String()
		if senderPeerIDStr != clusterPeerIDStr {
			return world.NewOpRefusedError(errors.Errorf("tx sender %s does not match cluster %s", senderPeerIDStr, clusterPeerIDStr))
		}

		// Store the replacement cluster peer ID.
		cluster.PeerId = peerIDStr
		changed = true
		bcs.SetBlock(cluster, true)
		return nil
	})
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	// Remove links to the previous cluster keypairs.
	oldKpKeys, err := identity_world.ListObjectKeypairs(ctx, worldHandle, clusterKey)
	if err != nil {
		return false, err
	}
	for _, oldKpKey := range oldKpKeys {
		err = worldHandle.DeleteGraphQuad(ctx, world.NewGraphQuadWithKeys(
			clusterKey,
			identity_world.PredObjectToKeypair.String(),
			oldKpKey,
			"",
		))
		if err != nil {
			return false, err
		}
	}

	// Link the replacement peer keypair to the cluster.
	_, _, err = identity_world.LinkObjectToKeypair(ctx, worldHandle, sender, clusterKey, peerID, "", nil)
	if err != nil {
		return false, err
	}

	// Finish after the cluster links are updated.
	return false, nil
}

// _ is a type assertion
var _ world.Operation = (*ClusterAssignPeerOp)(nil)
