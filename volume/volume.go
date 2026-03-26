package volume

import (
	"context"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/controller"
	block_gc "github.com/aperturerobotics/hydra/block/gc"
	block_store "github.com/aperturerobotics/hydra/block/store"
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
	// GetID returns the volume ID.
	// Usually this is derived from the peer ID and volume type.
	GetID() string

	// GetPeerID returns the volume peer ID.
	GetPeerID() peer.ID

	// GetPeer returns the Peer object.
	// If withPriv=false ensure that the Peer returned does not have the private key.
	GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error)

	// GetRefGraph returns the volume's GC reference graph.
	//
	// NOTE: May return nil if the volume does not support Garbage Collection.
	GetRefGraph() block_gc.RefGraphOps

	// Store indicates the volume is a hydra store.
	store.Store

	// Close closes the volume, returning any errors.
	Close() error

	// Delete closes the volume and removes the backing store.
	// After Delete returns, the volume's persistent data is gone.
	// Close is idempotent after Delete.
	Delete() error
}

// NewVolumeID constructs a new volume ID with a store type id and a peer id.
//
// storeTypeID should be like "hydra/volume/kvtxinmem"
func NewVolumeID(storeTypeID string, peerID peer.ID) string {
	return strings.Join([]string{
		storeTypeID,
		peerID.String(),
	}, "/")
}

// Controller is a volume controller.
type Controller interface {
	// Controller is the controllerbus controller interface.
	controller.Controller

	// GetVolume returns the controlled volume.
	// This may wait for the volume to be ready.
	GetVolume(ctx context.Context) (Volume, error)
	// BuildBucketAPI builds an API handle for the bucket ID in the volume.
	// Returns the handle & a release function, or (nil, nil, err).
	BuildBucketAPI(ctx context.Context, bucketID string) (bucket.BucketHandle, func(), error)
}

// ObjectStoreHandle is a object store API handle.
type ObjectStoreHandle interface {
	// GetID returns the object store ID.
	GetID() string
	// GetVolumeId returns the volume ID of the object store handle.
	GetVolumeId() string
	// GetObjectStore returns the object store.
	GetObjectStore() object.ObjectStore
}

// this assertion ensure LookupBlockStore matches Volume
var _ block_store.Store = ((Volume)(nil))
