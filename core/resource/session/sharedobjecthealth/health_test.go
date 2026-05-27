package sharedobjecthealth

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
)

type testStream struct {
	msgs chan *sobject.SharedObjectHealth
}

func newTestStream() *testStream {
	return &testStream{
		msgs: make(chan *sobject.SharedObjectHealth, 16),
	}
}

func (m *testStream) SendHealth(health *sobject.SharedObjectHealth) error {
	m.msgs <- health
	return nil
}

func recvHealth(
	t *testing.T,
	msgs <-chan *sobject.SharedObjectHealth,
) *sobject.SharedObjectHealth {
	t.Helper()

	select {
	case health := <-msgs:
		if health == nil {
			t.Fatal("expected health payload")
		}
		return health
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shared object health")
		return nil
	}
}

func TestStreamWatchableSendsLoadingThenLifecycle(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	healthCtr := ccontainer.NewCContainer[*sobject.SharedObjectHealth](nil)
	strm := newTestStream()
	errCh := make(chan error, 1)
	go func() {
		errCh <- StreamWatchable(ctx, strm, healthCtr)
	}()

	initial := recvHealth(t, strm.msgs)
	if initial.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected initial loading status, got %v", initial.GetStatus())
	}

	readyHealth := Ready()
	healthCtr.SetValue(readyHealth)
	ready := recvHealth(t, strm.msgs)
	if ready.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatalf("expected ready status, got %v", ready.GetStatus())
	}

	closedHealth := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
		"block not found",
	)
	healthCtr.SetValue(closedHealth)

	closed := recvHealth(t, strm.msgs)
	if closed.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed status, got %v", closed.GetStatus())
	}
	if closed.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND {
		t.Fatalf("expected block-not-found reason, got %v", closed.GetCommonReason())
	}

	cancel()
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("StreamWatchable() = %v, want context canceled", err)
	}
}

// _ is a type assertion
var _ Sender = (*testStream)(nil)
