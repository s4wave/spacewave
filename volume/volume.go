package volume

import (
	"context"
	"crypto"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
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

// Volume stores data with an associated peer ID.
type Volume interface {
	// GetID returns the volume ID, should be derived from the peer ID.
	GetID() string

	// GetPeerID returns the volume peer ID.
	GetPeerID() peer.ID

	// GetPeer returns the Peer object.
	// If withPriv=false ensure that the Peer returned does not have the private key.
	GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error)

	// Store indicates the volume is a hydra store.
	store.Store

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
func NewVolumeInfo(ctx context.Context, ci *controller.Info, vol Volume) (*VolumeInfo, error) {
	peerID := vol.GetPeerID().Pretty()
	peerInfo, err := vol.GetPeer(ctx, false)
	if err != nil {
		return nil, err
	}
	peerPub := peerInfo.GetPubKey()

	pkPem, err := keypem.MarshalPubKeyPem(peerPub)
	if err != nil {
		return nil, err
	}

	return &VolumeInfo{
		VolumeId:       vol.GetID(),
		PeerId:         peerID,
		PeerPub:        string(pkPem),
		ControllerInfo: ci.Clone(),
	}, nil
}

// Validate validates the VolumeInfo object.
func (i *VolumeInfo) Validate() error {
	peerID, err := i.ParsePeerID()
	if err == nil && len(peerID) == 0 {
		err = peer.ErrEmptyPeerID
	}
	if err != nil {
		return err
	}
	if _, err := i.ParseToPeer(); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer ID.
func (i *VolumeInfo) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(i.GetPeerId())
}

// ParsePeerPub parses the public key.
func (i *VolumeInfo) ParsePeerPub() (crypto.PublicKey, error) {
	return confparse.ParsePublicKey(i.GetPeerPub())
}

// ParseToPeer parses the fields and builds the corresponding Peer.
func (i *VolumeInfo) ParseToPeer() (peer.Peer, error) {
	return confparse.ParsePeer("", i.GetPeerPub(), i.GetPeerId())
}
