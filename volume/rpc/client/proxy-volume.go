package volume_rpc_client

import (
	"context"
	"errors"

	"github.com/aperturerobotics/bifrost/peer"
	rpc_block "github.com/aperturerobotics/hydra/block/rpc"
	rpc_block_client "github.com/aperturerobotics/hydra/block/rpc/client"
	rpc_bucket "github.com/aperturerobotics/hydra/bucket/store/rpc"
	rpc_bucket_client "github.com/aperturerobotics/hydra/bucket/store/rpc/client"
	rpc_mqueue "github.com/aperturerobotics/hydra/mqueue/rpc"
	rpc_mqueue_client "github.com/aperturerobotics/hydra/mqueue/rpc/client"
	rpc_object "github.com/aperturerobotics/hydra/object/rpc"
	rpc_object_client "github.com/aperturerobotics/hydra/object/rpc/client"
	"github.com/aperturerobotics/hydra/volume"
	volume_rpc "github.com/aperturerobotics/hydra/volume/rpc"
	volume_store "github.com/aperturerobotics/hydra/volume/store"
	"github.com/aperturerobotics/bifrost/crypto"
)

// ProxyVolume implements a volume backed by a ProxyVolume service.
type ProxyVolume struct {
	*rpc_block_client.BlockStore
	*rpc_bucket_client.BucketStore
	*rpc_object_client.ObjectStore
	*rpc_mqueue_client.MqueueStore

	// client is the client to use
	client volume_rpc.SRPCProxyVolumeClient
	// volInfo is the volume info
	volInfo *volume.VolumeInfo
	// volPeer is the parsed volume peer public key & id
	volPeer peer.Peer
}

// NewProxyVolume constructs a new ProxyVolume.
func NewProxyVolume(
	volInfo *volume.VolumeInfo,
	proxyVolumeClient volume_rpc.SRPCProxyVolumeClient,
	blockStoreClient rpc_block.SRPCBlockStoreClient,
	bucketStoreClient rpc_bucket.SRPCBucketStoreClient,
	objectStoreClient rpc_object.SRPCObjectStoreClient,
	mqueueStoreClient rpc_mqueue.SRPCMqueueStoreClient,
) (*ProxyVolume, error) {
	volPeer, err := volInfo.ParseToPeer()
	if err != nil {
		return nil, err
	}

	return &ProxyVolume{
		BlockStore:  rpc_block_client.NewBlockStore(blockStoreClient, volInfo.GetHashType(), false),
		BucketStore: rpc_bucket_client.NewBucketStore(bucketStoreClient),
		ObjectStore: rpc_object_client.NewObjectStore(objectStoreClient),
		MqueueStore: rpc_mqueue_client.NewMqueueStore(mqueueStoreClient),

		client:  proxyVolumeClient,
		volInfo: volInfo,
		volPeer: volPeer,
	}, nil
}

// GetID returns the volume ID, should be derived from the peer ID.
func (v *ProxyVolume) GetID() string {
	return v.volInfo.GetVolumeId()
}

// GetPeerID returns the volume peer ID.
func (v *ProxyVolume) GetPeerID() peer.ID {
	return v.volPeer.GetPeerID()
}

// GetVolumeClient returns the proxy volume client.
func (v *ProxyVolume) GetVolumeClient() volume_rpc.SRPCProxyVolumeClient {
	return v.client
}

// GetPeer returns the Peer object.
// If withPriv=false ensure that the Peer returned does not have the private key.
func (v *ProxyVolume) GetPeer(ctx context.Context, withPriv bool) (peer.Peer, error) {
	if !withPriv {
		return v.volPeer, nil
	}

	resp, err := v.client.GetPeerPriv(ctx, &volume_rpc.GetPeerPrivRequest{})
	if err == nil {
		err = resp.Validate()
	}
	if err != nil {
		return nil, err
	}

	privKey, err := resp.ParsePrivKey()
	if err != nil {
		return nil, err
	}

	return peer.NewPeer(privKey)
}

// LoadPeerPriv attempts to load the volume private key.
// May return nil if there is no key stored.
// May return ErrPrivKeyUnavailable
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

// _ is a type assertion
var (
	_ volume.Volume      = ((*ProxyVolume)(nil))
	_ volume_store.Store = ((*ProxyVolume)(nil))
)
