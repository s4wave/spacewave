package volume_rpc_client

import (
	"context"
	"errors"
	"sync"

	rpc_block "github.com/s4wave/spacewave/db/block/rpc"
	rpc_block_client "github.com/s4wave/spacewave/db/block/rpc/client"
	rpc_bucket "github.com/s4wave/spacewave/db/bucket/store/rpc"
	rpc_bucket_client "github.com/s4wave/spacewave/db/bucket/store/rpc/client"
	"github.com/s4wave/spacewave/db/coord"
	rpc_object "github.com/s4wave/spacewave/db/object/rpc"
	rpc_object_client "github.com/s4wave/spacewave/db/object/rpc/client"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	volume_store "github.com/s4wave/spacewave/db/volume/store"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// ProxyVolume implements a volume backed by a ProxyVolume service.
type ProxyVolume struct {
	*rpc_block_client.BlockStore
	*rpc_bucket_client.BucketStore
	*rpc_object_client.ObjectStore
	coord.Coordinator

	// client is the client to use
	client volume_rpc.SRPCProxyVolumeClient
	// volInfo is the volume info
	volInfo *volume.VolumeInfo
	// volPeer is the parsed volume peer public key & id
	volPeer peer.Peer
	// holdsRoots caches whether the remote volume holds roots, read once.
	holdsRoots func() bool
}

// NewProxyVolume constructs a new ProxyVolume.
func NewProxyVolume(
	volInfo *volume.VolumeInfo,
	proxyVolumeClient volume_rpc.SRPCProxyVolumeClient,
	blockStoreClient rpc_block.SRPCBlockStoreClient,
	bucketStoreClient rpc_bucket.SRPCBucketStoreClient,
	objectStoreClient rpc_object.SRPCObjectStoreClient,
) (*ProxyVolume, error) {
	// Parse the remote volume identity before constructing its storage facade.
	volPeer, err := volInfo.ParseToPeer()
	if err != nil {
		return nil, err
	}

	// Build the storage facade, asking whether the remote volume holds roots
	// once on first use.
	v := &ProxyVolume{
		BlockStore:  rpc_block_client.NewBlockStore(blockStoreClient, volInfo.ResolveHashType(), false),
		BucketStore: rpc_bucket_client.NewBucketStore(bucketStoreClient),
		ObjectStore: rpc_object_client.NewObjectStore(objectStoreClient),
		Coordinator: NewCoordinator(proxyVolumeClient),

		client:  proxyVolumeClient,
		volInfo: volInfo,
		volPeer: volPeer,
	}
	v.holdsRoots = sync.OnceValue(v.loadHoldsRoots)
	return v, nil
}

// GetID returns the volume ID, should be derived from the peer ID.
func (v *ProxyVolume) GetID() string {
	return v.volInfo.GetVolumeId()
}

// GetPeerID returns the volume peer ID.
func (v *ProxyVolume) GetPeerID() peer.ID {
	return v.volPeer.GetPeerID()
}

// GetStorageStats returns storage usage statistics via RPC.
func (v *ProxyVolume) GetStorageStats(ctx context.Context) (*volume.StorageStats, error) {
	resp, err := v.client.GetStorageStats(ctx, &volume_rpc.GetStorageStatsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetStorageStats(), nil
}

// DelegatesGC marks the volume as a view of storage collected by the controller
// of the served volume.
func (v *ProxyVolume) DelegatesGC() {}

// GetVolumeClient returns the proxy volume client.
func (v *ProxyVolume) GetVolumeClient() volume_rpc.SRPCProxyVolumeClient {
	return v.client
}

// GetPeer returns the Peer object.
// If withPriv=false ensure that the Peer returned does not have the private key.
func (v *ProxyVolume) GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error) {
	// Serve the cached volume peer when its private key is not requested.
	if !withPriv {
		return v.volPeer, nil
	}

	// Read and validate the remote volume private-key response.
	resp, err := v.client.GetPeerPriv(ctx, &volume_rpc.GetPeerPrivRequest{})
	if err == nil {
		err = resp.Validate()
	}
	if err != nil {
		return nil, err
	}

	// Decode the remote private key before constructing the volume peer.
	privKey, err := resp.ParsePrivKey()
	if err != nil {
		return nil, err
	}

	return peer.NewPeer(privKey)
}

// LoadPeerPriv attempts to load the volume private key.
// May return nil if there is no key stored.
// May return ErrPrivKeyUnavailable.
func (v *ProxyVolume) LoadPeerPriv(ctx context.Context) (crypto.PrivKey, error) {
	p, err := v.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	return p.GetPrivKey(ctx)
}

// StorePeerPriv overwrites the volume's stored private key.
func (v *ProxyVolume) StorePeerPriv(ctx context.Context, pkey crypto.PrivKey) error {
	return errors.New("cannot update proxy volume private key")
}

// Execute executes the volume store.
func (v *ProxyVolume) Execute(ctx context.Context) error {
	// no-op: if adding something here: call Execute in proxy-volume-controller.go
	return nil
}

// Close closes the volume, returning any errors.
func (v *ProxyVolume) Close() error {
	// no-op
	return nil
}

// Delete closes the volume and removes the backing store.
// ProxyVolume does not own a backing store, so this is a no-op.
func (v *ProxyVolume) Delete() error {
	return nil
}

// _ is a type assertion
var (
	_ volume.Volume      = (*ProxyVolume)(nil)
	_ volume_store.Store = (*ProxyVolume)(nil)
)
