package volume_rpc_server

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	rpc_gc "github.com/aperturerobotics/hydra/block/gc/rpc"
	rpc_gc_server "github.com/aperturerobotics/hydra/block/gc/rpc/server"
	rpc_block "github.com/aperturerobotics/hydra/block/rpc"
	rpc_block_server "github.com/aperturerobotics/hydra/block/rpc/server"
	rpc_bucket "github.com/aperturerobotics/hydra/bucket/store/rpc"
	rpc_bucket_server "github.com/aperturerobotics/hydra/bucket/store/rpc/server"
	rpc_mqueue "github.com/aperturerobotics/hydra/mqueue/rpc"
	rpc_mqueue_server "github.com/aperturerobotics/hydra/mqueue/rpc/server"
	rpc_object "github.com/aperturerobotics/hydra/object/rpc"
	rpc_object_server "github.com/aperturerobotics/hydra/object/rpc/server"
	"github.com/aperturerobotics/hydra/volume"
	volume_rpc "github.com/aperturerobotics/hydra/volume/rpc"
	"github.com/aperturerobotics/starpc/srpc"
)

// ProxyVolume implements the ProxyVolume service with a Volume.
type ProxyVolume struct {
	*rpc_block_server.BlockStore
	*rpc_bucket_server.BucketStore
	*rpc_object_server.ObjectStore
	*rpc_mqueue_server.MqueueStore
	*rpc_gc_server.RefGraph

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
		RefGraph:    rpc_gc_server.NewRefGraph(vol.GetRefGraph()),

		vol:           vol,
		exposePrivKey: exposePrivKey,
	}
}

// RegisterProxyVolume registers all ProxyVolume services.
func RegisterProxyVolume(mux srpc.Mux, proxyVol *ProxyVolume) error {
	return RegisterProxyVolumeWithPrefix(mux, proxyVol, "")
}

// RegisterProxyVolumeWithPrefix registers all ProxyVolume services with a service id prefix.
func RegisterProxyVolumeWithPrefix(mux srpc.Mux, proxyVol *ProxyVolume, prefix string) error {
	// register ProxyVolume
	if err := mux.Register(volume_rpc.NewSRPCProxyVolumeHandler(
		proxyVol,
		prefix+volume_rpc.SRPCProxyVolumeServiceID,
	)); err != nil {
		return err
	}
	// register BlockStore
	if err := mux.Register(rpc_block.NewSRPCBlockStoreHandler(
		proxyVol,
		prefix+rpc_block.SRPCBlockStoreServiceID,
	)); err != nil {
		return err
	}
	// register BucketStore
	if err := mux.Register(rpc_bucket.NewSRPCBucketStoreHandler(
		proxyVol,
		prefix+rpc_bucket.SRPCBucketStoreServiceID,
	)); err != nil {
		return err
	}
	// register ObjectStore
	if err := mux.Register(rpc_object.NewSRPCObjectStoreHandler(
		proxyVol,
		prefix+rpc_object.SRPCObjectStoreServiceID,
	)); err != nil {
		return err
	}
	// register MqueueStore
	if err := mux.Register(rpc_mqueue.NewSRPCMqueueStoreHandler(
		proxyVol,
		prefix+rpc_mqueue.SRPCMqueueStoreServiceID,
	)); err != nil {
		return err
	}
	// register RefGraph
	if err := mux.Register(rpc_gc.NewSRPCRefGraphHandler(
		proxyVol,
		prefix+rpc_gc.SRPCRefGraphServiceID,
	)); err != nil {
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
	req *volume_rpc.GetVolumeInfoRequest,
) (*volume_rpc.GetVolumeInfoResponse, error) {
	volInfo, err := volume.NewVolumeInfo(ctx, nil, v.vol)
	if err != nil {
		return nil, err
	}
	return &volume_rpc.GetVolumeInfoResponse{
		VolumeInfo: volInfo,
	}, nil
}

// GetPeerPriv returns the private key for the volume (if enabled).
func (v *ProxyVolume) GetPeerPriv(
	ctx context.Context,
	req *volume_rpc.GetPeerPrivRequest,
) (*volume_rpc.GetPeerPrivResponse, error) {
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
	return volume_rpc.NewGetPeerPrivResponse(peerPriv)
}

// GetStorageStats returns storage usage statistics for the volume.
func (v *ProxyVolume) GetStorageStats(
	ctx context.Context,
	req *volume_rpc.GetStorageStatsRequest,
) (*volume_rpc.GetStorageStatsResponse, error) {
	stats, err := v.vol.GetStorageStats(ctx)
	if err != nil {
		return nil, err
	}
	return &volume_rpc.GetStorageStatsResponse{StorageStats: stats}, nil
}

// _ is a type assertion
var (
	_ volume_rpc.SRPCProxyVolumeServer = ((*ProxyVolume)(nil))
	_ rpc_block.SRPCBlockStoreServer   = ((*ProxyVolume)(nil))
	_ rpc_bucket.SRPCBucketStoreServer = ((*ProxyVolume)(nil))
	_ rpc_object.SRPCObjectStoreServer = ((*ProxyVolume)(nil))
	_ rpc_mqueue.SRPCMqueueStoreServer = ((*ProxyVolume)(nil))
	_ rpc_gc.SRPCRefGraphServer        = ((*ProxyVolume)(nil))
)
