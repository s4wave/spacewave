package volume_rpc_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	rpc_gc "github.com/s4wave/spacewave/db/block/gc/rpc"
	rpc_gc_server "github.com/s4wave/spacewave/db/block/gc/rpc/server"
	rpc_block "github.com/s4wave/spacewave/db/block/rpc"
	rpc_block_server "github.com/s4wave/spacewave/db/block/rpc/server"
	rpc_bucket "github.com/s4wave/spacewave/db/bucket/store/rpc"
	rpc_bucket_server "github.com/s4wave/spacewave/db/bucket/store/rpc/server"
	"github.com/s4wave/spacewave/db/coord"
	rpc_object "github.com/s4wave/spacewave/db/object/rpc"
	rpc_object_server "github.com/s4wave/spacewave/db/object/rpc/server"
	"github.com/s4wave/spacewave/db/volume"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	"github.com/s4wave/spacewave/net/peer"
)

// ProxyVolume implements the ProxyVolume service with a Volume.
type ProxyVolume struct {
	*rpc_block_server.BlockStore
	*rpc_bucket_server.BucketStore
	*rpc_object_server.ObjectStore
	*rpc_gc_server.RefGraph

	// vol is the volume
	vol volume.Volume
	// coordinatorLeases owns remote write leases acquired through this service.
	coordinatorLeases *coordinatorLeases
	// exposePrivKey controls if we allow exposing the private key
	exposePrivKey bool
}

// NewProxyVolume constructs a new ProxyVolume.
func NewProxyVolume(ctx context.Context, vol volume.Volume, exposePrivKey bool) *ProxyVolume {
	return &ProxyVolume{
		BlockStore:  rpc_block_server.NewBlockStore(vol),
		BucketStore: rpc_bucket_server.NewBucketStore(vol),
		ObjectStore: rpc_object_server.NewObjectStore(ctx, vol),
		RefGraph:    rpc_gc_server.NewRefGraph(vol.GetRefGraph()),

		vol:               vol,
		coordinatorLeases: newCoordinatorLeases(),
		exposePrivKey:     exposePrivKey,
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

// GetCoordinatorCapability reports the remote coordinator capability.
func (v *ProxyVolume) GetCoordinatorCapability(
	ctx context.Context,
	req *volume_rpc.GetCoordinatorCapabilityRequest,
) (*volume_rpc.GetCoordinatorCapabilityResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	scope := req.GetScope().ToCoordScope()
	if scope.VolumeID == "" {
		scope.VolumeID = v.vol.GetID()
	}

	capability, err := v.vol.Capability(ctx, scope)
	if err != nil {
		return nil, err
	}
	if capability == nil {
		capability = &coord.Capability{
			Supported:      false,
			FallbackReason: coord.FallbackReasonUnsupported,
		}
	}
	capability.VolumeID = scope.VolumeID
	capability.ObjectStoreID = scope.ObjectStoreID
	if capability.Supported {
		capability.Backend = coord.BackendKindRPC
	} else if capability.Backend == "" {
		capability.Backend = coord.BackendKindRPC
	}

	return &volume_rpc.GetCoordinatorCapabilityResponse{
		Capability: volume_rpc.NewCoordinatorCapability(capability),
	}, nil
}

// WatchCoordinatorEvents streams remote coordinator events.
func (v *ProxyVolume) WatchCoordinatorEvents(
	req *volume_rpc.WatchCoordinatorEventsRequest,
	strm volume_rpc.SRPCProxyVolume_WatchCoordinatorEventsStream,
) error {
	ctx := strm.Context()
	scope := req.GetScope().ToCoordScope()
	if scope.VolumeID == "" {
		scope.VolumeID = v.vol.GetID()
	}

	watch, err := v.vol.Watch(ctx, scope, req.GetAfterGeneration())
	if err != nil {
		return err
	}
	defer watch.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watch.Events():
			if !ok {
				return nil
			}
			if err := strm.Send(&volume_rpc.WatchCoordinatorEventsResponse{
				Event: volume_rpc.NewCoordinatorEvent(event),
			}); err != nil {
				return err
			}
		}
	}
}

// GetCoordinatorSnapshot returns the current remote coordinator snapshot.
func (v *ProxyVolume) GetCoordinatorSnapshot(
	ctx context.Context,
	req *volume_rpc.GetCoordinatorSnapshotRequest,
) (*volume_rpc.GetCoordinatorSnapshotResponse, error) {
	scope := req.GetScope().ToCoordScope()
	if scope.VolumeID == "" {
		scope.VolumeID = v.vol.GetID()
	}

	snapshot, err := v.vol.Snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	return &volume_rpc.GetCoordinatorSnapshotResponse{
		Snapshot: volume_rpc.NewCoordinatorSnapshot(snapshot),
	}, nil
}

// TryAcquireCoordinatorWriteLease attempts to acquire the remote write lease.
func (v *ProxyVolume) TryAcquireCoordinatorWriteLease(
	req *volume_rpc.TryAcquireCoordinatorWriteLeaseRequest,
	strm volume_rpc.SRPCProxyVolume_TryAcquireCoordinatorWriteLeaseStream,
) error {
	ctx := strm.Context()
	scope := req.GetScope().ToCoordScope()
	if scope.VolumeID == "" {
		scope.VolumeID = v.vol.GetID()
	}

	lease, acquired, err := v.vol.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		return err
	}
	if !acquired {
		return strm.SendAndClose(&volume_rpc.AcquireCoordinatorWriteLeaseResponse{
			Acquired: false,
		})
	}
	leaseID := v.coordinatorLeases.add(lease)
	defer v.coordinatorLeases.release(context.Background(), leaseID)

	if err := strm.Send(&volume_rpc.AcquireCoordinatorWriteLeaseResponse{
		LeaseId:  leaseID,
		Acquired: true,
	}); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

// WaitAcquireCoordinatorWriteLease waits to acquire the remote write lease.
func (v *ProxyVolume) WaitAcquireCoordinatorWriteLease(
	req *volume_rpc.WaitAcquireCoordinatorWriteLeaseRequest,
	strm volume_rpc.SRPCProxyVolume_WaitAcquireCoordinatorWriteLeaseStream,
) error {
	ctx := strm.Context()
	scope := req.GetScope().ToCoordScope()
	if scope.VolumeID == "" {
		scope.VolumeID = v.vol.GetID()
	}

	lease, err := v.vol.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return err
	}
	leaseID := v.coordinatorLeases.add(lease)
	defer v.coordinatorLeases.release(context.Background(), leaseID)

	if err := strm.Send(&volume_rpc.AcquireCoordinatorWriteLeaseResponse{
		LeaseId:  leaseID,
		Acquired: true,
	}); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

// RefreshCoordinatorWriteLease refreshes a remote write lease.
func (v *ProxyVolume) RefreshCoordinatorWriteLease(
	ctx context.Context,
	req *volume_rpc.CoordinatorWriteLeaseRequest,
) (*volume_rpc.CoordinatorWriteLeaseSnapshotResponse, error) {
	lease, err := v.coordinatorLeases.get(req.GetLeaseId())
	if err != nil {
		return nil, err
	}
	snapshot, err := lease.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	return &volume_rpc.CoordinatorWriteLeaseSnapshotResponse{
		Snapshot: volume_rpc.NewCoordinatorSnapshot(snapshot),
	}, nil
}

// PublishCoordinatorWriteLease publishes a remote write lease event.
func (v *ProxyVolume) PublishCoordinatorWriteLease(
	ctx context.Context,
	req *volume_rpc.PublishCoordinatorWriteLeaseRequest,
) (*volume_rpc.CoordinatorWriteLeaseSnapshotResponse, error) {
	lease, err := v.coordinatorLeases.get(req.GetLeaseId())
	if err != nil {
		return nil, err
	}
	snapshot, err := lease.Publish(ctx, req.GetEvent().ToCoordEvent())
	if err != nil {
		return nil, err
	}
	return &volume_rpc.CoordinatorWriteLeaseSnapshotResponse{
		Snapshot: volume_rpc.NewCoordinatorSnapshot(snapshot),
	}, nil
}

// ReleaseCoordinatorWriteLease releases a remote write lease.
func (v *ProxyVolume) ReleaseCoordinatorWriteLease(
	ctx context.Context,
	req *volume_rpc.CoordinatorWriteLeaseRequest,
) (*volume_rpc.ReleaseCoordinatorWriteLeaseResponse, error) {
	if err := v.coordinatorLeases.release(context.Background(), req.GetLeaseId()); err != nil {
		return nil, err
	}
	return &volume_rpc.ReleaseCoordinatorWriteLeaseResponse{}, nil
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
	_ rpc_gc.SRPCRefGraphServer        = ((*ProxyVolume)(nil))
)
