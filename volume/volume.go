package volume

import (
	"context"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/object"
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

	// GetID returns the volume ID, should be derived from the peer ID.
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
	// The handles are valid while ctx is valid.
	BuildBucketAPI(
		ctx context.Context,
		bucketID string,
	) (BucketHandle, error)
}

// BucketHandle is a bucket API handle.
// All calls use the bucket handle context.
type BucketHandle interface {
	// GetContext returns the handle context.
	GetContext() context.Context
	// GetID returns the bucket ID.
	GetID() string
	// GetVolumeId returns the volume ID of the bucket handle.
	GetVolumeId() string
	// GetExists returns if the handle is valid. If false, the bucket does not
	// exist in the volume, and all block calls will not work.
	GetExists() bool
	// GetBucketConfig returns the bucket configuration in use.
	// May be nil if the bucket does not exist in the volume.
	GetBucketConfig() *bucket.Config

	// GetBucket returns the bucket object.
	// May be nil if the handle is not valid.
	GetBucket() bucket.Bucket

	// Close closes the bucket handle.
	// May be called many times.
	// Does not block.
	Close()
}

// ObjectStoreHandle is a object store API handle.
type ObjectStoreHandle interface {
	// GetContext returns the handle context.
	GetContext() context.Context
	// GetID returns the object store ID.
	GetID() string
	// GetVolumeId returns the volume ID of the object store handle.
	GetVolumeId() string
	// GetError returns any error opening the object store.
	GetError() error

	// GetObjectStore returns the object store.
	// May be nil if the handle is not valid.
	GetObjectStore() object.ObjectStore

	// Close closes the bucket handle.
	// May be called many times.
	// Does not block.
	Close()
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
