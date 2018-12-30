package volume

import (
	"context"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/store"
	"github.com/sirupsen/logrus"
)

// Constructor constructs a volume with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
) (Volume, error)

// Volume is a storage device attached to the network.
type Volume interface {
	// Peer indicates the volume has a peer identity.
	peer.Peer
	// Store indicates the volume is a hydra store.
	store.Store

	// GetID returns the volume IDn of the
	// peer ID, volume type, etc for regular-expression filtering.
	GetID() string

	// Close closes the volume, returning any errors.
	Close() error
}

// Controller is a volume controller.
type Controller interface {
	// Controller is the controllerbus controller interface.
	controller.Controller

	// GetVolume returns the controlled volume.
	// This may wait for the volume to be ready.
	GetVolume(ctx context.Context) (Volume, error)
	// BuildBucketAPI builds an API handle for the bucket ID in the volume.
	// If the bucket is not found, should monitor in case it is created.
	// The handles are valid while ctx is valid.
	BuildBucketAPI(
		ctx context.Context,
		bucketID string,
		cb func(b bucket.Bucket, added bool),
	) error
}

// NewVolumeInfo constructs volume info from a volume.
func NewVolumeInfo(ci controller.Info, vol Volume) (*VolumeInfo, error) {
	peerID := vol.GetPeerID().Pretty()
	peerPub := vol.GetPubKey()

	pkPem, err := keypem.MarshalPubKeyPem(peerPub)
	if err != nil {
		return nil, err
	}

	return &VolumeInfo{
		VolumeId:       vol.GetID(),
		PeerId:         peerID,
		PeerPub:        string(pkPem),
		ControllerInfo: &ci,
	}, nil
}
