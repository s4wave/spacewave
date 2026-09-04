//go:build !js

package stream_datagram

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/link"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/tests"
	"github.com/s4wave/spacewave/net/stream"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	stream_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

// The endpoint peers have no shared simulated LAN. Only their signaling peer
// bridges the LANs, forcing the application stream through native WebRTC.
func TestDatagramsOverWebRTC(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	g := graph.NewGraph()
	source, signal, target := tests.AddPeer(t, g), tests.AddPeer(t, g), tests.AddPeer(t, g)
	signal.AddFactory(func(b bus.Bus) controller.Factory { return signaling_server.NewFactory(b) })
	signal.AddConfig("signaling-server", &signaling_server.Config{Server: &stream_server.Config{
		PeerIds: []string{signal.GetPeerID().String()}, ProtocolIds: []string{string(signaling.ProtocolID)},
	}})
	for _, p := range []*graph.Peer{source, target} {
		p.AddFactory(func(b bus.Bus) controller.Factory { return signaling_client.NewFactory(b) })
		p.AddFactory(func(b bus.Bus) controller.Factory { return webrtc.NewFactory(b) })
		p.AddConfig("signaling-client", &signaling_client.Config{
			SignalingId: "signal", Client: &stream_client.Config{ServerPeerIds: []string{signal.GetPeerID().String()}},
		})
		p.AddConfig("webrtc", &webrtc.Config{SignalingId: "signal", AllPeers: true, BlockPeers: []string{signal.GetPeerID().String()}})
		lan := graph.AddLAN(g)
		lan.AddPeer(g, p)
		lan.AddPeer(g, signal)
	}
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	sim := tests.InitSimulator(t, ctx, logrus.NewEntry(logger), g)
	defer sim.Close()
	mounted, release, err := link.OpenStreamWithPeerEx(ctx,
		sim.GetPeerByID(source.GetPeerID()).GetTestbed().Bus, stream_echo.DefaultProtocolID,
		source.GetPeerID(), target.GetPeerID(), 0, stream.OpenOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if mounted.GetPeerID() != target.GetPeerID() {
		t.Fatal("wrong authenticated remote peer")
	}
	endpoint, client := udpSocket(t), udpSocket(t)
	result := make(chan error, 1)
	go func() { result <- ForwardLocal(ctx, endpoint, mounted.GetStream()) }()
	for _, payload := range [][]byte{{}, []byte("initial game handshake"), bytes.Repeat([]byte{0x5a}, 1400), bytes.Repeat([]byte{0xa5}, 8192)} {
		if _, err := client.WriteToUDP(payload, endpoint.LocalAddr().(*net.UDPAddr)); err != nil {
			t.Fatal(err)
		}
		packet := make([]byte, MaxPacketSize)
		n, _, err := client.ReadFromUDP(packet)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(payload, packet[:n]) {
			t.Fatalf("UDP echo differs for %d byte payload", len(payload))
		}
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WebRTC datagram forwarding did not stop")
	}
}
