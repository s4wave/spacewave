package websocket

import (
	"context"
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

func buildTestbed(t *testing.T, ctx context.Context) (*testbed.Testbed, *logrus.Entry) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	return tb, le
}

func execPeer(
	ctx context.Context,
	t *testing.T,
	tb *testbed.Testbed,
	conf *Config,
) (*transport_controller.Controller, *WebSocket, directive.Reference) {
	peerID, err := peer.IDFromPrivateKey(tb.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}

	if conf == nil {
		conf = &Config{}
	}
	conf.TransportPeerId = peerID.String()

	tpc, _, tpRef, err := loader.WaitExecControllerRunningTyped[*transport_controller.Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	tpt, err := tpc.GetTransport(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	return tpc, tpt.(*WebSocket), tpRef
}

// TestWebSocketLink tests creating a WebSocket link between two peers on
// localhost and holding the connection open for 12 seconds.
func TestWebSocketLink(t *testing.T) {
	ctx := t.Context()

	tb1, le1 := buildTestbed(t, ctx)
	le1 = le1.WithField("testbed", 0)
	tb2, le2 := buildTestbed(t, ctx)
	le2 = le2.WithField("testbed", 1)

	const listenAddr = "127.0.0.1:19384"

	// Peer 1 listens on localhost.
	_, ws1, ws1Ref := execPeer(ctx, t, tb1, &Config{
		ListenAddr: listenAddr,
		Verbose:    true,
	})
	defer ws1Ref.Release()
	peerID1 := ws1.GetPeerID()

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

	le1.Infof("peer 1: %s", peerID1.String())
	le2.Infof("peer 2: %s", peerID2.String())

	// Establish a link from peer 2 to peer 1.
	lnk, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", peerID1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	le1.Infof("link established: %v", lnk.GetLinkUUID())

	// Open an echo stream to verify the link works.
	ms, err := lnk.OpenMountedStream(ctx, stream_echo.DefaultProtocolID, stream.OpenOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ms.GetStream().Close()

	data := []byte("hello websocket")
	if _, err := ms.GetStream().Write(data); err != nil {
		t.Fatal(err.Error())
	}
	buf := make([]byte, len(data)*2)
	n, err := ms.GetStream().Read(buf)
	if err != nil {
		t.Fatal(err.Error())
	}
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
	n, err = ms.GetStream().Read(buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(buf[:n]) != string(data) {
		t.Fatalf("final echo mismatch: got %q, want %q", buf[:n], data)
	}
	le1.Info("final echo verified, test passed")
}
