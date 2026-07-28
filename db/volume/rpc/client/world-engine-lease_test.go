package volume_rpc_client

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
)

func TestWorldEngineLeaseStreamTerminationSignalsLoss(t *testing.T) {
	rpc := &terminatingLeaseRPCClient{}
	provider := newWorldEngineLeaseProvider(
		volume_rpc.NewSRPCProxyVolumeClient(rpc),
	)
	lease, err := provider.AcquireWorldEngineLease(context.Background(), "world-object")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("stream termination did not close Done")
	}
	if !errors.Is(lease.Err(), io.EOF) {
		t.Fatalf("lease error = %v, want io.EOF", lease.Err())
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("release after stream loss: %v", err)
	}
	if calls := rpc.unaryCalls.Load(); calls != 0 {
		t.Fatalf("release confirmation calls after stream loss = %d, want 0", calls)
	}
}

func TestWorldEngineLeaseAcquisitionHonorsContextCancellation(t *testing.T) {
	provider := newWorldEngineLeaseProvider(
		volume_rpc.NewSRPCProxyVolumeClient(&blockedLeaseRPCClient{}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := provider.AcquireWorldEngineLease(ctx, "world-object")
		done <- err
	}()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("acquisition error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled acquisition did not return promptly")
	}
}

func TestWorldEngineLeaseReleaseCancelsStreamBeforeBoundedUnary(t *testing.T) {
	rpc := &unresponsiveReleaseRPCClient{
		unaryStarted:   make(chan struct{}),
		unaryRelease:   make(chan struct{}),
		streamCanceled: make(chan struct{}),
	}
	t.Cleanup(func() { close(rpc.unaryRelease) })

	provider := newWorldEngineLeaseProvider(
		volume_rpc.NewSRPCProxyVolumeClient(rpc),
	)
	lease, err := provider.AcquireWorldEngineLease(context.Background(), "world-object")
	if err != nil {
		t.Fatal(err)
	}

	releaseDone := make(chan error, 1)
	go func() {
		releaseDone <- lease.Release()
	}()

	select {
	case <-rpc.streamCanceled:
	case <-time.After(time.Second):
		t.Fatal("lease stream was not canceled before release returned")
	}

	select {
	case <-rpc.unaryStarted:
	case <-time.After(time.Second):
		t.Fatal("release confirmation was not attempted")
	}

	select {
	case err := <-releaseDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("release error = %v, want context deadline", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("release did not return promptly")
	}
}

type unresponsiveReleaseRPCClient struct {
	unaryStarted   chan struct{}
	unaryRelease   chan struct{}
	streamCanceled chan struct{}
}

func (c *unresponsiveReleaseRPCClient) ExecCall(
	_ context.Context,
	_ string,
	_ string,
	_ srpc.Message,
	_ srpc.Message,
) error {
	close(c.unaryStarted)
	<-c.unaryRelease
	return context.Canceled
}

func (c *unresponsiveReleaseRPCClient) NewStream(
	ctx context.Context,
	_ string,
	_ string,
	_ srpc.Message,
) (srpc.Stream, error) {
	go func() {
		<-ctx.Done()
		close(c.streamCanceled)
	}()
	return &respondingLeaseStream{ctx: ctx}, nil
}

type respondingLeaseStream struct {
	ctx      context.Context
	received bool
}

func (s *respondingLeaseStream) Context() context.Context { return s.ctx }
func (*respondingLeaseStream) MsgSend(srpc.Message) error { return nil }
func (s *respondingLeaseStream) MsgRecv(m srpc.Message) error {
	if s.received {
		<-s.ctx.Done()
		return s.ctx.Err()
	}
	response, ok := m.(*volume_rpc.AcquireWorldEngineLeaseResponse)
	if !ok {
		return errors.New("unexpected lease response type")
	}
	s.received = true
	response.LeaseId = "lease-1"
	response.Acquired = true
	return nil
}
func (*respondingLeaseStream) CloseSend() error { return nil }
func (*respondingLeaseStream) Close() error     { return nil }

var _ srpc.Client = (*unresponsiveReleaseRPCClient)(nil)

type blockedLeaseRPCClient struct{}

func (*blockedLeaseRPCClient) ExecCall(context.Context, string, string, srpc.Message, srpc.Message) error {
	return errors.New("unexpected unary call")
}

func (*blockedLeaseRPCClient) NewStream(ctx context.Context, _ string, _ string, _ srpc.Message) (srpc.Stream, error) {
	return &blockedLeaseStream{ctx: ctx}, nil
}

type blockedLeaseStream struct {
	ctx context.Context
}

func (s *blockedLeaseStream) Context() context.Context { return s.ctx }
func (*blockedLeaseStream) MsgSend(srpc.Message) error { return nil }
func (s *blockedLeaseStream) MsgRecv(srpc.Message) error {
	<-s.ctx.Done()
	return s.ctx.Err()
}
func (*blockedLeaseStream) CloseSend() error { return nil }
func (*blockedLeaseStream) Close() error     { return nil }

type terminatingLeaseRPCClient struct {
	unaryCalls atomic.Int32
}

func (c *terminatingLeaseRPCClient) ExecCall(
	context.Context,
	string,
	string,
	srpc.Message,
	srpc.Message,
) error {
	c.unaryCalls.Add(1)
	return nil
}

func (*terminatingLeaseRPCClient) NewStream(
	ctx context.Context,
	_ string,
	_ string,
	_ srpc.Message,
) (srpc.Stream, error) {
	return &terminatingLeaseStream{ctx: ctx}, nil
}

type terminatingLeaseStream struct {
	ctx      context.Context
	received bool
}

func (s *terminatingLeaseStream) Context() context.Context { return s.ctx }
func (*terminatingLeaseStream) MsgSend(srpc.Message) error { return nil }
func (s *terminatingLeaseStream) MsgRecv(m srpc.Message) error {
	if s.received {
		return io.EOF
	}
	response, ok := m.(*volume_rpc.AcquireWorldEngineLeaseResponse)
	if !ok {
		return errors.New("unexpected lease response type")
	}
	s.received = true
	response.LeaseId = "lease-1"
	response.Acquired = true
	return nil
}
func (*terminatingLeaseStream) CloseSend() error { return nil }
func (*terminatingLeaseStream) Close() error     { return nil }

var _ srpc.Client = (*terminatingLeaseRPCClient)(nil)
