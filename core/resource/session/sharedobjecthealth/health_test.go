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
	// Run each health-stream transition test independently.
	t.Parallel()

	// Create a cancelable lifecycle context for the stream.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the health stream with an empty watchable value and error channel.
	healthCtr := ccontainer.NewCContainer[*sobject.SharedObjectHealth](nil)
	strm := newTestStream()
	errCh := make(chan error, 1)
	go func() {
		errCh <- StreamWatchable(ctx, strm, healthCtr)
	}()

	// Receive the stream's initial health state.
	initial := recvHealth(t, strm.msgs)

	// Assert that an empty watchable value is reported as loading.
	if initial.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected initial loading status, got %v", initial.GetStatus())
	}

	// Publish ready health through the watchable account state.
	readyHealth := Ready()
	healthCtr.SetValue(readyHealth)

	// Receive the ready-state update from the stream.
	ready := recvHealth(t, strm.msgs)

	// Assert that the watch publishes the ready transition.
	if ready.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
		t.Fatalf("expected ready status, got %v", ready.GetStatus())
	}

	// Publish closed health with a durable block-not-found reason.
	closedHealth := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
		"block not found",
	)
	healthCtr.SetValue(closedHealth)

	// Receive the closed-state update from the stream.
	closed := recvHealth(t, strm.msgs)

	// Assert that the watch publishes the closed status.
	if closed.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed status, got %v", closed.GetStatus())
	}

	// Assert that the closed health retains its specific failure reason.
	if closed.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND {
		t.Fatalf("expected block-not-found reason, got %v", closed.GetCommonReason())
	}

	// Cancel the stream lifecycle after observing the health transitions.
	cancel()

	// Assert that the stream exits with the context cancellation result.
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("StreamWatchable() = %v, want context canceled", err)
	}
}

func TestStreamWatchableSendsLoadingBeforeCurrentTypedHealth(t *testing.T) {
	// Run the preloaded typed-health test independently.
	t.Parallel()

	// Create a cancelable lifecycle context for the stream.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the stream with an already-populated body health value.
	typedHealth := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
		"unsupported shared object type: weird.body",
	)
	healthCtr := ccontainer.NewCContainer[*sobject.SharedObjectHealth](typedHealth)
	strm := newTestStream()
	errCh := make(chan error, 1)
	go func() {
		errCh <- StreamWatchable(ctx, strm, healthCtr)
	}()

	// Receive the stream's loading state before its current value.
	initial := recvHealth(t, strm.msgs)

	// Assert that the watcher publishes loading first.
	if initial.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected initial loading status, got %v", initial.GetStatus())
	}

	// Receive the current typed health state after loading.
	current := recvHealth(t, strm.msgs)

	// Assert that the current state preserves its closed status.
	if current.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
		t.Fatalf("expected closed status, got %v", current.GetStatus())
	}

	// Assert that typed health preserves its body layer.
	if current.GetLayer() != sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY {
		t.Fatalf("expected body layer, got %v", current.GetLayer())
	}

	// Assert that typed health preserves its decode-failure reason.
	if current.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED {
		t.Fatalf("expected body-config reason, got %v", current.GetCommonReason())
	}

	// Assert that typed health preserves its source-data remediation hint.
	if current.GetRemediationHint() != sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA {
		t.Fatalf("expected repair-source-data remediation hint, got %v", current.GetRemediationHint())
	}

	// Assert that typed health preserves the original decode detail.
	if current.GetError() != "unsupported shared object type: weird.body" {
		t.Fatalf("expected typed health detail to survive, got %q", current.GetError())
	}

	// Cancel the stream after validating the preloaded health state.
	cancel()

	// Assert that the stream exits with the context cancellation result.
	if err := <-errCh; err != context.Canceled {
		t.Fatalf("StreamWatchable() = %v, want context canceled", err)
	}
}

// _ is a type assertion
var _ Sender = (*testStream)(nil)
