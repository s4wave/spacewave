package sobject_sync

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// peerConnectionEvent is one call of the peer connection observer.
type peerConnectionEvent struct {
	remoteID        peer.ID
	remoteTransport peer.ID
	connected       bool
	err             error
}

// TestPeerConnectionReportsStreamGap cuts the link between two devices holding
// a Space and checks that the observer reports the stream's end and its error,
// and that a new stream reports the reconnect and delivers the write made while
// the link was down.
func TestPeerConnectionReportsStreamGap(t *testing.T) {
	// An owner and a reader start from the same authenticated state.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const soID = "peer-connection-gap"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	events := make(chan peerConnectionEvent, 4)
	local.SetPeerConnectionObserver(func(remoteID, remoteTransport peer.ID, connected bool, err error) {
		events <- peerConnectionEvent{remoteID, remoteTransport, connected, err}
	})
	states, release, err := remote.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// connect runs both ends of a new stream and returns its link.
	connect := func() (net.Conn, <-chan error) {
		left, right := net.Pipe()
		t.Cleanup(func() { left.Close(); right.Close() })
		done := make(chan error, 2)
		go func() { done <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
		go func() { done <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
		return left, done
	}

	// next waits for the observer's next report.
	next := func() peerConnectionEvent {
		t.Helper()
		select {
		case event := <-events:
			if event.remoteID != remote.localObjectPeerID || event.remoteTransport != "transport-b" {
				t.Fatalf("observed peer %s over %s, want %s over transport-b", event.remoteID, event.remoteTransport, remote.localObjectPeerID)
			}
			return event
		case <-ctx.Done():
			t.Fatal("no peer connection report")
			return peerConnectionEvent{}
		}
	}

	// write adds an operation on the owner and waits for the reader to hold it.
	write := func(data string, done <-chan error) {
		t.Helper()
		if err := local.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
			writeSyncOp(t, soID, state, owner, data)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		target, err := local.soHost.GetHostState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
			return len(state.GetOps()) == len(target.GetOps()), nil
		}, done); err != nil {
			t.Fatal(err)
		}
	}

	// The stream reports its participant and delivers a write.
	link, done := connect()
	if event := next(); !event.connected || event.err != nil {
		t.Fatalf("first report = %+v, want connected", event)
	}
	write("before the cut", done)

	// Cutting the link ends the stream with its error.
	link.Close()
	if event := next(); event.connected || event.err == nil {
		t.Fatalf("report after cut = %+v, want disconnected with an error", event)
	}
	for range 2 {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("stream survived the cut")
		}
	}

	// A write made while apart arrives over the next stream.
	if err := local.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		writeSyncOp(t, soID, state, owner, "during the cut")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, done = connect()
	if event := next(); !event.connected || event.err != nil {
		t.Fatalf("report after reconnect = %+v, want connected", event)
	}
	write("after the cut", done)

	// Cancellation ends the stream and reports it.
	cancel()
	select {
	case event := <-events:
		if event.connected {
			t.Fatalf("report after cancel = %+v, want disconnected", event)
		}
	case <-time.After(time.Second):
		t.Fatal("no report after cancel")
	}
}
