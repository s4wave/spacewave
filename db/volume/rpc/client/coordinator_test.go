package volume_rpc_client

import (
	"errors"
	"io"
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
)

func TestCoordinatorLeaseDetectedLossContract(t *testing.T) {
	lease := &lease{
		cancel: func() {},
		done:   make(chan struct{}),
	}
	capability := &coord.Capability{Supported: true, DetectsLoss: true}
	conformance.CheckDetectedLoss(t, capability, lease, func() {
		lease.watchStream(func() error { return io.EOF })
	})
	if !errors.Is(lease.Err(), io.EOF) {
		t.Fatalf("lease error = %v, want EOF", lease.Err())
	}
}
