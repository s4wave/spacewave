package volume_rpc_client

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/coord"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
)

// Coordinator negotiates remote coordination capability for a ProxyVolume.
type Coordinator struct {
	client      volume_rpc.SRPCProxyVolumeClient
	unsupported *coord.UnsupportedCoordinator
}

// NewCoordinator constructs an RPC-backed coordinator facade.
func NewCoordinator(client volume_rpc.SRPCProxyVolumeClient) *Coordinator {
	return &Coordinator{
		client: client,
		unsupported: coord.NewUnsupportedCoordinator(
			coord.BackendKindRPC,
			coord.FallbackReasonUnsupported,
		),
	}
}

// Capability reports the server's structured remote coordination capability.
func (c *Coordinator) Capability(ctx context.Context, scope coord.Scope) (*coord.Capability, error) {
	resp, err := c.client.GetCoordinatorCapability(ctx, &volume_rpc.GetCoordinatorCapabilityRequest{
		Scope: volume_rpc.NewCoordinatorScope(scope),
	})
	if err != nil {
		return nil, err
	}
	capability := resp.GetCapability().ToCoordCapability()
	if capability == nil {
		return c.unsupported.Capability(ctx, scope)
	}
	return capability, nil
}

// Snapshot returns the remote coordination snapshot.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	resp, err := c.client.GetCoordinatorSnapshot(ctx, &volume_rpc.GetCoordinatorSnapshotRequest{
		Scope: volume_rpc.NewCoordinatorScope(scope),
	})
	if err != nil {
		return nil, err
	}
	return resp.GetSnapshot().ToCoordSnapshot(), nil
}

// Watch streams remote coordination events through the ProxyVolume service.
func (c *Coordinator) Watch(ctx context.Context, scope coord.Scope, afterGeneration uint64) (coord.Watch, error) {
	watchCtx, cancel := context.WithCancel(ctx)
	stream, err := c.client.WatchCoordinatorEvents(watchCtx, &volume_rpc.WatchCoordinatorEventsRequest{
		Scope:           volume_rpc.NewCoordinatorScope(scope),
		AfterGeneration: afterGeneration,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	watch := &watch{
		stream:  stream,
		cancel:  cancel,
		events:  make(chan coord.Event, 16),
		closing: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go watch.receive()
	return watch, nil
}

// TryAcquireWriteLease attempts to acquire the remote write lease.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	leaseCtx, cancel := context.WithCancel(context.Background())
	stream, err := c.client.TryAcquireCoordinatorWriteLease(leaseCtx, &volume_rpc.TryAcquireCoordinatorWriteLeaseRequest{
		Scope: volume_rpc.NewCoordinatorScope(scope),
	})
	if err != nil {
		cancel()
		return nil, false, err
	}
	resp, err := stream.Recv()
	if err != nil {
		cancel()
		return nil, false, err
	}
	if !resp.GetAcquired() {
		cancel()
		return nil, false, nil
	}
	return &lease{
		client:  c.client,
		cancel:  cancel,
		leaseID: resp.GetLeaseId(),
	}, true, nil
}

// WaitAcquireWriteLease waits to acquire the remote write lease.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	leaseCtx, cancel := context.WithCancel(ctx)
	stream, err := c.client.WaitAcquireCoordinatorWriteLease(leaseCtx, &volume_rpc.WaitAcquireCoordinatorWriteLeaseRequest{
		Scope: volume_rpc.NewCoordinatorScope(scope),
	})
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		cancel()
		return nil, err
	}
	return &lease{
		client:  c.client,
		cancel:  cancel,
		leaseID: resp.GetLeaseId(),
	}, nil
}

var _ coord.Coordinator = (*Coordinator)(nil)

type watch struct {
	stream  volume_rpc.SRPCProxyVolume_WatchCoordinatorEventsClient
	cancel  context.CancelFunc
	events  chan coord.Event
	closing chan struct{}
	done    chan struct{}

	closeOnce sync.Once
}

func (w *watch) Events() <-chan coord.Event {
	return w.events
}

func (w *watch) Close() error {
	w.closeOnce.Do(func() {
		w.cancel()
		close(w.closing)
		_ = w.stream.Close()
		<-w.done
	})
	return nil
}

func (w *watch) receive() {
	defer close(w.done)
	defer close(w.events)

	for {
		resp, err := w.stream.Recv()
		if err != nil {
			return
		}
		event := resp.GetEvent().ToCoordEvent()
		select {
		case w.events <- event:
		case <-w.closing:
			return
		}
	}
}

var _ coord.Watch = (*watch)(nil)

type lease struct {
	client  volume_rpc.SRPCProxyVolumeClient
	cancel  context.CancelFunc
	leaseID string
}

func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	resp, err := l.client.RefreshCoordinatorWriteLease(ctx, &volume_rpc.CoordinatorWriteLeaseRequest{
		LeaseId: l.leaseID,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetSnapshot().ToCoordSnapshot(), nil
}

func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	resp, err := l.client.PublishCoordinatorWriteLease(ctx, &volume_rpc.PublishCoordinatorWriteLeaseRequest{
		LeaseId: l.leaseID,
		Event:   volume_rpc.NewCoordinatorEvent(event),
	})
	if err != nil {
		return nil, err
	}
	return resp.GetSnapshot().ToCoordSnapshot(), nil
}

func (l *lease) Release(ctx context.Context) error {
	defer l.cancel()
	_, err := l.client.ReleaseCoordinatorWriteLease(ctx, &volume_rpc.CoordinatorWriteLeaseRequest{
		LeaseId: l.leaseID,
	})
	return err
}

var _ coord.WriteLease = (*lease)(nil)
