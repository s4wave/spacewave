package websocket

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// buildTestbed constructs a peer bus with the WebSocket transport factory.
func buildTestbed(t *testing.T, ctx context.Context) (*testbed.Testbed, *logrus.Entry) {
	// Configure the logger for the WebSocket transport testbed.
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create a network testbed with the WebSocket transport factory.
	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	return tb, le
}

// execPeer starts a peer transport and returns its retained controller reference.
func execPeer(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	conf *Config,
) (*transport_controller.Controller, *WebSocket, directive.Reference) {
	// Derive the WebSocket peer identity from the testbed key.
	t.Helper()
	peerID, err := peer.IDFromPrivateKey(tb.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Bind the WebSocket configuration to the testbed peer.
	if conf == nil {
		conf = &Config{}
	}
	conf.TransportPeerId = peerID.String()

	// Load the WebSocket controller and wait for its running state.
	tpc, _, tpRef, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Obtain the WebSocket transport from the running controller.
	tpt, err := tpc.GetTransport(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	return tpc, tpt.(*WebSocket), tpRef
}

// TestWebSocketLink tests creating a WebSocket link between two peers on
// localhost and holding the connection open for 12 seconds.
func TestWebSocketLink(t *testing.T) {
	// Bind the WebSocket test to the testing lifecycle.
	ctx := t.Context()

	// Create two peer testbeds for an isolated local WebSocket listener.
	tb1, le1 := buildTestbed(t, ctx)
	le1 = le1.WithField("testbed", 0)
	tb2, le2 := buildTestbed(t, ctx)
	le2 = le2.WithField("testbed", 1)

	// Serve peer 1 through an owned, ephemeral HTTP endpoint.
	_, ws1, ws1Ref := execPeer(ctx, t, tb1, &Config{Verbose: true})
	defer ws1Ref.Release()
	peerID1 := ws1.GetPeerID()
	server := httptest.NewServer(ws1)
	t.Cleanup(server.Close)
	listenAddr := server.Listener.Addr().String()

	// Peer 2 dials peer 1 over websocket.
	_, ws2, ws2Ref := execPeer(ctx, t, tb2, &Config{
		Verbose: true,
		Dialers: map[string]*dialer.DialerOpts{
			peerID1.String(): {
				Address: "ws://" + listenAddr,
			},
		},
	})
	defer ws2Ref.Release()
	peerID2 := ws2.GetPeerID()

	// Report the identities participating in the WebSocket link.
	le1.Infof("peer 1: %s", peerID1.String())
	le2.Infof("peer 2: %s", peerID2.String())

	// Establish a link from peer 2 to peer 1.
	lnk, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", peerID1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	// Record the established WebSocket link for the echo exchange.
	le1.Infof("link established: %v", lnk.GetLinkUUID())

	// Open an echo stream to verify the link works.
	ms, err := lnk.OpenMountedStream(ctx, stream_echo.DefaultProtocolID, stream.OpenOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ms.GetStream().Close()

	// Send the first payload through the WebSocket echo stream.
	data := []byte("hello websocket")
	if _, err := ms.GetStream().Write(data); err != nil {
		t.Fatal(err.Error())
	}

	// Receive the first echoed payload over the WebSocket link.
	buf := make([]byte, len(data)*2)
	n, err := ms.GetStream().Read(buf)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the first echo before holding the WebSocket connection open.
	if string(buf[:n]) != string(data) {
		t.Fatalf("echo mismatch: got %q, want %q", buf[:n], data)
	}
	le1.Info("echo verified, holding connection open for 12 seconds")

	// Hold the connection open for 12 seconds.
	select {
	case <-time.After(12 * time.Second):
		le1.Info("12 seconds elapsed, connection held successfully")
	case <-ctx.Done():
		t.Fatal("context canceled before 12 seconds elapsed")
	}

	// Final echo to confirm the link is still alive.
	data = []byte("still alive")
	if _, err := ms.GetStream().Write(data); err != nil {
		t.Fatal(err.Error())
	}

	// Receive the final echo after the WebSocket connection stayed open.
	n, err = ms.GetStream().Read(buf)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the WebSocket link still returns the complete payload.
	if string(buf[:n]) != string(data) {
		t.Fatalf("final echo mismatch: got %q, want %q", buf[:n], data)
	}
	le1.Info("final echo verified, test passed")
}

// TestListenHTTPStopsOnCancellation checks cancellation before serving starts.
func TestListenHTTPStopsOnCancellation(t *testing.T) {
	// Build a real transport before canceling its listener context.
	tb, _ := buildTestbed(t, t.Context())
	_, transport, ref := execPeer(t.Context(), t, tb, &Config{})
	t.Cleanup(ref.Release)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan error, 1)
	go func() { done <- transport.ListenHTTP(ctx, "127.0.0.1:0") }()

	// A canceled transport must not retain its HTTP listener.
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("canceled listener returned %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP listener survived transport cancellation")
	}
}
