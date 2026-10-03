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
	// Register the volume service under the requested prefix.
	// register ProxyVolume
	if err := mux.Register(volume_rpc.NewSRPCProxyVolumeHandler(
		proxyVol,
		prefix+volume_rpc.SRPCProxyVolumeServiceID,
	)); err != nil {
		return err
	}

	// Register the block store alongside the volume service.
	// register BlockStore
	if err := mux.Register(rpc_block.NewSRPCBlockStoreHandler(
		proxyVol,
		prefix+rpc_block.SRPCBlockStoreServiceID,
	)); err != nil {
		return err
	}

	// Register the bucket store alongside the volume service.
	// register BucketStore
	if err := mux.Register(rpc_bucket.NewSRPCBucketStoreHandler(
		proxyVol,
		prefix+rpc_bucket.SRPCBucketStoreServiceID,
	)); err != nil {
		return err
	}

	// Register the object store alongside the volume service.
	// register ObjectStore
	if err := mux.Register(rpc_object.NewSRPCObjectStoreHandler(
		proxyVol,
		prefix+rpc_object.SRPCObjectStoreServiceID,
	)); err != nil {
		return err
	}

	// Register the reference graph alongside the volume service.
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
	// Stop the capability request when its context has ended.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Bind the capability scope to the proxied volume.
	scope := req.GetScope().ToCoordScope()
	scope.VolumeID = v.vol.GetID()

	// Describe coordinator support through the RPC backend.
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
	// Bind the event watch scope to the proxied volume.
	ctx := strm.Context()
	scope := req.GetScope().ToCoordScope()
	scope.VolumeID = v.vol.GetID()

	// Register the volume watch for the lifetime of the event stream.
	watch, err := v.vol.Watch(ctx, scope, req.GetAfterGeneration())
	if err != nil {
		return err
	}
	defer watch.Close()

	// Acknowledge registration before the client can initiate work whose
	// non-generational events must be observed by this watch.
	if err := strm.Send(&volume_rpc.WatchCoordinatorEventsResponse{}); err != nil {
		return err
	}

	// Forward coordinator events until the watch or stream ends.
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
	// Bind the snapshot scope to the proxied volume.
	scope := req.GetScope().ToCoordScope()
	scope.VolumeID = v.vol.GetID()

	// Read the coordinator snapshot from the volume.
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
	// Bind the lease attempt to the proxied volume and stream.
	ctx := strm.Context()
	scope := req.GetScope().ToCoordScope()
	scope.VolumeID = v.vol.GetID()

	// Attempt the volume write lease without waiting for another writer.
	lease, acquired, err := v.vol.TryAcquireWriteLease(ctx, scope)
	if err != nil {
		return err
	}
	if !acquired {
		return strm.SendAndClose(&volume_rpc.AcquireCoordinatorWriteLeaseResponse{
			Acquired: false,
		})
	}

	// Track the acquired lease until this stream ends.
	leaseID, err := v.coordinatorLeases.add(lease)
	if err != nil {
		_ = lease.Release(context.Background())
		return err
	}
	defer v.coordinatorLeases.release(context.Background(), leaseID)

	// Publish the lease identifier and wait for the lease or stream to end.
	if err := strm.Send(&volume_rpc.AcquireCoordinatorWriteLeaseResponse{
		LeaseId:  leaseID,
		Acquired: true,
	}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-lease.Done():
		return lease.Err()
	}
}

// WaitAcquireCoordinatorWriteLease waits to acquire the remote write lease.
func (v *ProxyVolume) WaitAcquireCoordinatorWriteLease(
	req *volume_rpc.WaitAcquireCoordinatorWriteLeaseRequest,
	strm volume_rpc.SRPCProxyVolume_WaitAcquireCoordinatorWriteLeaseStream,
) error {
	// Bind the waiting lease request to the proxied volume and stream.
	ctx := strm.Context()
	scope := req.GetScope().ToCoordScope()
	scope.VolumeID = v.vol.GetID()

	// Wait for the volume write lease to become available.
	lease, err := v.vol.WaitAcquireWriteLease(ctx, scope)
	if err != nil {
		return err
	}

	// Track the acquired lease until this stream ends.
	leaseID, err := v.coordinatorLeases.add(lease)
	if err != nil {
		_ = lease.Release(context.Background())
		return err
	}
	defer v.coordinatorLeases.release(context.Background(), leaseID)

	// Publish the lease identifier and wait for the lease or stream to end.
	if err := strm.Send(&volume_rpc.AcquireCoordinatorWriteLeaseResponse{
		LeaseId:  leaseID,
		Acquired: true,
	}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-lease.Done():
		return lease.Err()
	}
}

// RefreshCoordinatorWriteLease refreshes a remote write lease.
func (v *ProxyVolume) RefreshCoordinatorWriteLease(
	ctx context.Context,
	req *volume_rpc.CoordinatorWriteLeaseRequest,
) (*volume_rpc.CoordinatorWriteLeaseSnapshotResponse, error) {
	// Find the tracked lease before refreshing its coordinator snapshot.
	lease, err := v.coordinatorLeases.get(req.GetLeaseId())
	if err != nil {
		return nil, err
	}

	// Refresh the lease and return its current coordinator snapshot.
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
	// Find the tracked lease before publishing the coordinator event.
	lease, err := v.coordinatorLeases.get(req.GetLeaseId())
	if err != nil {
		return nil, err
	}

	// Publish the event through the lease and return its coordinator snapshot.
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
	// Require private key exposure before accessing the volume peer.
	if !v.exposePrivKey {
		return nil, peer.ErrNoPrivKey
	}

	// Obtain the volume peer with access to its private key.
	peerWithPriv, err := v.vol.GetPeer(ctx, true)
	if err != nil {
		return nil, err
	}

	// Read the peer private key for the RPC response.
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
	_ volume_rpc.SRPCProxyVolumeServer = (*ProxyVolume)(nil)
	_ rpc_block.SRPCBlockStoreServer   = (*ProxyVolume)(nil)
	_ rpc_bucket.SRPCBucketStoreServer = (*ProxyVolume)(nil)
	_ rpc_object.SRPCObjectStoreServer = (*ProxyVolume)(nil)
	_ rpc_gc.SRPCRefGraphServer        = (*ProxyVolume)(nil)
)
