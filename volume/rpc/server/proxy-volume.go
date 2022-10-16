package volume_rpc_server

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	rpc_block "github.com/aperturerobotics/hydra/block/rpc"
	rpc_block_server "github.com/aperturerobotics/hydra/block/rpc/server"
	rpc_bucket "github.com/aperturerobotics/hydra/bucket/rpc"
	rpc_bucket_server "github.com/aperturerobotics/hydra/bucket/rpc/server"
	rpc_mqueue "github.com/aperturerobotics/hydra/mqueue/rpc"
	rpc_mqueue_server "github.com/aperturerobotics/hydra/mqueue/rpc/server"
	rpc_object "github.com/aperturerobotics/hydra/object/rpc"
	rpc_object_server "github.com/aperturerobotics/hydra/object/rpc/server"
	"github.com/aperturerobotics/hydra/volume"
	rpc_volume "github.com/aperturerobotics/hydra/volume/rpc"
	"github.com/aperturerobotics/starpc/srpc"
)

// ProxyVolume implements the ProxyVolume service with a Volume.
type ProxyVolume struct {
	*rpc_block_server.BlockStore
	*rpc_bucket_server.BucketStore
	*rpc_object_server.ObjectStore
	*rpc_mqueue_server.MqueueStore

	// vol is the volume
	vol volume.Volume
	// exposePrivKey controls if we allow exposing the private key
	exposePrivKey bool
}

// NewProxyVolume constructs a new ProxyVolume.
func NewProxyVolume(ctx context.Context, vol volume.Volume, exposePrivKey bool) *ProxyVolume {
	return &ProxyVolume{
		BlockStore:  rpc_block_server.NewBlockStore(vol),
		BucketStore: rpc_bucket_server.NewBucketStore(vol),
		ObjectStore: rpc_object_server.NewObjectStore(ctx, vol),
		MqueueStore: rpc_mqueue_server.NewMqueueStore(vol),

		vol:           vol,
		exposePrivKey: exposePrivKey,
	}
}

// RegisterProxyVolume registers all ProxyVolume services.
func RegisterProxyVolume(mux srpc.Mux, proxyVol *ProxyVolume) error {
	if err := rpc_volume.SRPCRegisterProxyVolume(mux, proxyVol); err != nil {
		return err
	}
	if err := rpc_block.SRPCRegisterBlockStore(mux, proxyVol); err != nil {
		return err
	}
	if err := rpc_object.SRPCRegisterObjectStore(mux, proxyVol); err != nil {
		return err
	}
	if err := rpc_mqueue.SRPCRegisterMqueueStore(mux, proxyVol); err != nil {
		return err
	}
	return nil
}

// GetVolume returns the underlying volume.
func (v *ProxyVolume) GetVolume() volume.Volume {
	return v.vol
}

// GetVolumeInfo returns the volume information.
func (v *ProxyVolume) GetVolumeInfo(
	ctx context.Context,
	req *rpc_volume.GetVolumeInfoRequest,
) (*rpc_volume.GetVolumeInfoResponse, error) {
	volInfo, err := volume.NewVolumeInfo(ctx, nil, v.vol)
	if err != nil {
		return nil, err
	}
	return &rpc_volume.GetVolumeInfoResponse{
		VolumeInfo: volInfo,
	}, nil
}

// GetPeerPriv returns the private key for the volume (if enabled).
func (v *ProxyVolume) GetPeerPriv(
	ctx context.Context,
	req *rpc_volume.GetPeerPrivRequest,
) (*rpc_volume.GetPeerPrivResponse, error) {
	if !v.exposePrivKey {
		return nil, peer.ErrNoPrivKey
	}

	peerWithPriv, err := v.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}
	peerPriv, err := peerWithPriv.GetPrivKey(ctx)
	if err != nil {
		return nil, err
	}
	return rpc_volume.NewGetPeerPrivResponse(peerPriv)
}

// _ is a type assertion
var (
	_ rpc_volume.SRPCProxyVolumeServer = ((*ProxyVolume)(nil))
	_ rpc_block.SRPCBlockStoreServer   = ((*ProxyVolume)(nil))
	_ rpc_bucket.SRPCBucketStoreServer = ((*ProxyVolume)(nil))
	_ rpc_object.SRPCObjectStoreServer = ((*ProxyVolume)(nil))
	_ rpc_mqueue.SRPCMqueueStoreServer = ((*ProxyVolume)(nil))
)
