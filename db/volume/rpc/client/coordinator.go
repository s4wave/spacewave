package volume_rpc_client

import (
	"context"
	"sync"
	"sync/atomic"

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
		return nil, normalizeCoordError(err)
	}
	capability := resp.GetCapability().ToCoordCapability()
	if capability == nil {
		return c.unsupported.Capability(ctx, scope)
	}
	if capability.Supported {
		// The acquire stream carries lease liveness, so the client detects
		// loss regardless of the server backend.
		capability.DetectsLoss = true
	}
	return capability, nil
}

// Snapshot returns the remote coordination snapshot.
func (c *Coordinator) Snapshot(ctx context.Context, scope coord.Scope) (*coord.Snapshot, error) {
	resp, err := c.client.GetCoordinatorSnapshot(ctx, &volume_rpc.GetCoordinatorSnapshotRequest{
		Scope: volume_rpc.NewCoordinatorScope(scope),
	})
	if err != nil {
		return nil, normalizeCoordError(err)
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
		return nil, normalizeCoordError(err)
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
//
// The acquire stream stays open for the lease lifetime: the server releases
// the lease when the stream ends, and the client reports loss through
// WriteLease.Done when the stream ends first.
func (c *Coordinator) TryAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, bool, error) {
	leaseCtx, cancel := context.WithCancel(context.Background())
	stopAcquireCancel := context.AfterFunc(ctx, cancel)
	stream, err := c.client.TryAcquireCoordinatorWriteLease(leaseCtx, &volume_rpc.TryAcquireCoordinatorWriteLeaseRequest{
		Scope: volume_rpc.NewCoordinatorScope(scope),
	})
	if err != nil {
		stopAcquireCancel()
		cancel()
		return nil, false, normalizeCoordError(err)
	}
	resp, err := stream.Recv()
	if err != nil {
		stopAcquireCancel()
		cancel()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		return nil, false, err
	}
	if !resp.GetAcquired() {
		stopAcquireCancel()
		cancel()
		return nil, false, nil
	}
	if !stopAcquireCancel() {
		// ctx ended during acquisition: the stream is already canceled and
		// the server releases the lease on stream teardown.
		cancel()
		return nil, false, ctx.Err()
	}
	l := &lease{
		client:  c.client,
		cancel:  cancel,
		leaseID: resp.GetLeaseId(),
		done:    make(chan struct{}),
	}
	go l.watchStream(func() error {
		_, err := stream.Recv()
		return err
	})
	return l, true, nil
}

// WaitAcquireWriteLease waits to acquire the remote write lease.
func (c *Coordinator) WaitAcquireWriteLease(ctx context.Context, scope coord.Scope) (coord.WriteLease, error) {
	leaseCtx, cancel := context.WithCancel(context.Background())
	stopAcquireCancel := context.AfterFunc(ctx, cancel)
	stream, err := c.client.WaitAcquireCoordinatorWriteLease(leaseCtx, &volume_rpc.WaitAcquireCoordinatorWriteLeaseRequest{
		Scope: volume_rpc.NewCoordinatorScope(scope),
	})
	if err != nil {
		stopAcquireCancel()
		cancel()
		return nil, normalizeCoordError(err)
	}
	resp, err := stream.Recv()
	if err != nil {
		stopAcquireCancel()
		cancel()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, normalizeCoordError(err)
	}
	if !stopAcquireCancel() {
		cancel()
		return nil, ctx.Err()
	}
	l := &lease{
		client:  c.client,
		cancel:  cancel,
		leaseID: resp.GetLeaseId(),
		done:    make(chan struct{}),
	}
	go l.watchStream(func() error {
		_, err := stream.Recv()
		return err
	})
	return l, nil
}

var _ coord.Coordinator = (*Coordinator)(nil)

type watch struct {
	stream  volume_rpc.SRPCProxyVolume_WatchCoordinatorEventsClient
	cancel  context.CancelFunc
	events  chan coord.Event
	closing chan struct{}
	done    chan struct{}

	closeOnce atomic.Bool
}

func (w *watch) Events() <-chan coord.Event {
	return w.events
}

func (w *watch) Close() error {
	if w.closeOnce.CompareAndSwap(false, true) {
		w.cancel()
		close(w.closing)
		_ = w.stream.Close()
		<-w.done
	}
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
	done    chan struct{}

	// mtx guards released and lossErr; done closes exactly once when either
	// released or lossErr transitions from its zero value.
	mtx      sync.Mutex
	released bool
	lossErr  error
}

// Done returns a channel closed when the lease is released or the acquire
// stream ends while the lease is held.
func (l *lease) Done() <-chan struct{} {
	return l.done
}

// Err returns the stream error that lost the lease, or nil for a held or
// cleanly released lease.
func (l *lease) Err() error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.lossErr
}

// watchStream marks the lease lost when the acquire stream ends before
// Release cancels it.
func (l *lease) watchStream(recv func() error) {
	for {
		if err := recv(); err != nil {
			l.markLost(err)
			return
		}
	}
}

func (l *lease) markLost(err error) {
	l.mtx.Lock()
	if l.released || l.lossErr != nil {
		l.mtx.Unlock()
		return
	}
	l.lossErr = err
	close(l.done)
	l.mtx.Unlock()
	l.cancel()
}

func (l *lease) Refresh(ctx context.Context) (*coord.Snapshot, error) {
	resp, err := l.client.RefreshCoordinatorWriteLease(ctx, &volume_rpc.CoordinatorWriteLeaseRequest{
		LeaseId: l.leaseID,
	})
	if err != nil {
		return nil, normalizeCoordError(err)
	}
	return resp.GetSnapshot().ToCoordSnapshot(), nil
}

func (l *lease) Publish(ctx context.Context, event coord.Event) (*coord.Snapshot, error) {
	resp, err := l.client.PublishCoordinatorWriteLease(ctx, &volume_rpc.PublishCoordinatorWriteLeaseRequest{
		LeaseId: l.leaseID,
		Event:   volume_rpc.NewCoordinatorEvent(event),
	})
	if err != nil {
		return nil, normalizeCoordError(err)
	}
	return resp.GetSnapshot().ToCoordSnapshot(), nil
}

func (l *lease) Release(context.Context) error {
	l.mtx.Lock()
	if l.released {
		l.mtx.Unlock()
		return nil
	}
	l.released = true
	lost := l.lossErr != nil
	if !lost {
		close(l.done)
	}
	l.mtx.Unlock()

	// The stream and unary request are two idempotent release paths. Cancel the
	// stream first so the server releases the lease even if transport teardown
	// wins the race with the unary request.
	l.cancel()
	if lost {
		return nil
	}
	_, err := l.client.ReleaseCoordinatorWriteLease(context.Background(), &volume_rpc.CoordinatorWriteLeaseRequest{
		LeaseId: l.leaseID,
	})
	return normalizeCoordError(err)
}

// normalizeCoordError restores coordinator sentinel errors after SRPC has
// transported them as text.
func normalizeCoordError(err error) error {
	if err == nil {
		return nil
	}
	switch err.Error() {
	case coord.ErrUnsupported.Error():
		return coord.ErrUnsupported
	case coord.ErrLeaseReleased.Error():
		return coord.ErrLeaseReleased
	case coord.ErrStaleGeneration.Error():
		return coord.ErrStaleGeneration
	case coord.ErrScopeEmpty.Error():
		return coord.ErrScopeEmpty
	default:
		return err
	}
}

var _ coord.WriteLease = (*lease)(nil)
