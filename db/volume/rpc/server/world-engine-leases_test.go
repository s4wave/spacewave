package volume_rpc_server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	"github.com/sirupsen/logrus"
)

func TestWorldEngineLeasesReleaseIsIdempotent(t *testing.T) {
	lease := &countingWorldEngineLease{}
	leases := newWorldEngineLeases()
	leaseID := leases.add(lease)

	if err := leases.release(leaseID); err != nil {
		t.Fatalf("first release failed: %v", err)
	}
	if err := leases.release(leaseID); err != nil {
		t.Fatalf("second release failed: %v", err)
	}
	if lease.releases != 1 {
		t.Fatalf("lease release count = %d, want 1", lease.releases)
	}
}

type countingWorldEngineLease struct {
	releases int
	done     chan struct{}
}

func (l *countingWorldEngineLease) Done() <-chan struct{} {
	if l.done == nil {
		l.done = make(chan struct{})
	}
	return l.done
}

func (*countingWorldEngineLease) Err() error {
	return nil
}

func (l *countingWorldEngineLease) Release() error {
	l.releases++
	if l.done != nil {
		close(l.done)
	}
	return nil
}

var _ volume.WorldEngineLease = (*countingWorldEngineLease)(nil)

// lossInjectedVolume wraps a real volume and hands out a losable lease.
type lossInjectedVolume struct {
	volume.Volume
	lease *lossInjectedLease
}

func (v *lossInjectedVolume) AcquireWorldEngineLease(
	ctx context.Context,
	key string,
) (volume.WorldEngineLease, error) {
	return v.lease, nil
}

type lossInjectedLease struct {
	done chan struct{}
	err  error
}

func (l *lossInjectedLease) Done() <-chan struct{} { return l.done }

func (l *lossInjectedLease) Err() error { return l.err }

func (l *lossInjectedLease) Release() error { return nil }

func (l *lossInjectedLease) lose(err error) {
	l.err = err
	close(l.done)
}

// heldLeaseStream stubs the acquisition stream for the proxy handler.
type heldLeaseStream struct {
	srpc.Stream
	ctx  context.Context
	sent chan *volume_rpc.AcquireWorldEngineLeaseResponse
}

func (s *heldLeaseStream) Context() context.Context { return s.ctx }

func (s *heldLeaseStream) Send(resp *volume_rpc.AcquireWorldEngineLeaseResponse) error {
	s.sent <- resp
	return nil
}

func (s *heldLeaseStream) SendAndClose(resp *volume_rpc.AcquireWorldEngineLeaseResponse) error {
	s.sent <- resp
	return nil
}

// TestTryAcquireWorldEngineLeaseStreamEndsOnLeaseLoss verifies that losing
// the backing lease terminates the held acquisition stream with the loss
// error instead of waiting for the RPC context.
func TestTryAcquireWorldEngineLeaseStreamEndsOnLeaseLoss(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	backing, err := volume_kvtxinmem.NewKVTxInmem(ctx, le, &volume_kvtxinmem.Config{})
	if err != nil {
		t.Fatal(err)
	}
	lease := &lossInjectedLease{done: make(chan struct{})}
	proxy := NewProxyVolume(ctx, &lossInjectedVolume{Volume: backing, lease: lease}, false)

	strm := &heldLeaseStream{
		ctx:  ctx,
		sent: make(chan *volume_rpc.AcquireWorldEngineLeaseResponse, 1),
	}
	handlerDone := make(chan error, 1)
	go func() {
		handlerDone <- proxy.TryAcquireWorldEngineLease(
			&volume_rpc.TryAcquireWorldEngineLeaseRequest{Key: "world-object"},
			strm,
		)
	}()

	select {
	case resp := <-strm.sent:
		if !resp.GetAcquired() {
			t.Fatal("lease was not acquired")
		}
	case <-time.After(time.Second):
		t.Fatal("acquisition response was not sent")
	}

	lossErr := errors.New("lease renewal failed")
	lease.lose(lossErr)

	select {
	case err := <-handlerDone:
		if !errors.Is(err, lossErr) {
			t.Fatalf("handler error = %v, want lease loss error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not end on lease loss")
	}
}
