package stream_srpc_server_lookup

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	stream_srpc "github.com/s4wave/spacewave/net/stream/srpc"
	"github.com/sirupsen/logrus"
)

// testProtocolID is the protocol ID used for the lookup server in tests.
const testProtocolID = "bifrost/stream/srpc/server/lookup/test"

// TestLookup tests the lookup SRPC server controller end-to-end.
//
// p0 (client) dials p1 (server) over a LAN. p1 runs the lookup controller
// which resolves echo RPCs via LookupRpcService on the bus.
func TestLookup(t *testing.T) {
	ctx := t.Context()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	g := graph.NewGraph()
	p0 := addPeer(t, g)
	p1 := addPeer(t, g)

	lan := graph.AddLAN(g)
	lan.AddPeer(g, p0)
	lan.AddPeer(g, p1)

	sim := initSimulator(t, ctx, le, g, simulate.WithVerbose())

	sp0 := sim.GetPeerByID(p0.GetPeerID())
	sp1 := sim.GetPeerByID(p1.GetPeerID())

	// Verify connectivity first.
	if err := simulate.TestConnectivity(ctx, sp0, sp1); err != nil {
		t.Fatal(err.Error())
	}

	tb1 := sp1.GetTestbed()

	// Register echo service on p1's bus via RpcServiceController.
	mux := srpc.NewMux()
	if err := mux.Register(echo.NewSRPCEchoerHandler(echo.NewEchoServer(nil), echo.SRPCEchoerServiceID)); err != nil {
		t.Fatal(err.Error())
	}
	rpcCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test/echo-rpc", semver.MustParse("0.0.1"), "echo rpc service"),
		bifrost_rpc.NewRpcServiceBuilder(mux),
		nil,
		true,
		nil,
		nil,
		nil,
	)
	relRpc, err := tb1.Bus.AddController(ctx, rpcCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relRpc()

	// Start the lookup controller on p1 listening on testProtocolID.
	lookupCtrl, err := NewController(tb1.Bus, le, &Config{
		PeerIds:     []string{sp1.GetPeerID().String()},
		ProtocolIds: []string{testProtocolID},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	relLookup, err := tb1.Bus.AddController(ctx, lookupCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relLookup()

	// From p0, open an SRPC client to p1 on testProtocolID.
	tb0 := sp0.GetTestbed()
	openStream := stream_srpc.NewOpenStreamFunc(
		tb0.Bus,
		testProtocolID,
		sp0.GetPeerID(),
		sp1.GetPeerID(),
		0,
	)
	client := srpc.NewClient(openStream)

	// Call the echo service through the lookup controller.
	echoClient := echo.NewSRPCEchoerClient(client)
	msg := "hello from lookup test"
	resp, err := echoClient.Echo(ctx, &echo.EchoMsg{Body: msg})
	if err != nil {
		t.Fatal(err.Error())
	}
	if resp.GetBody() != msg {
		t.Fatal(errors.Errorf("expected %q but got %q", msg, resp.GetBody()))
	}
}
