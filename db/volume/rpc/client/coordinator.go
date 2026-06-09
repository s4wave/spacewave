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

// Snapshot returns ErrUnsupported until remote coordination snapshots are exposed.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	return c.unsupported.Snapshot(ctx, scope)
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

// TryAcquireWriteLease returns ErrUnsupported until remote write leases are exposed.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	return c.unsupported.TryAcquireWriteLease(ctx, scope)
}

// WaitAcquireWriteLease returns ErrUnsupported until remote write leases are exposed.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	return c.unsupported.WaitAcquireWriteLease(ctx, scope)
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
