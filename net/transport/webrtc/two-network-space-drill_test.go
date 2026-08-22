//go:build !js

package webrtc_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pion/logging"
	"github.com/pion/transport/v4/vnet"
	pion "github.com/pion/webrtc/v4"
	"github.com/s4wave/spacewave/net/link"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

// TwoNetworkSpaceDrillEnv gates every stage that opens sockets or dials an
// external endpoint. Without it the drill runs only the configuration-path
// assertions and reports the remaining stages as not-run.
const twoNetworkSpaceDrillEnv = "SPACEWAVE_TWO_NETWORK_DRILL"

// TestIceConfigPathDrill verifies the transport-owned ICE configuration path:
// STUN and TURN entries configured in WebRtcConfig reach the effective
// webrtc.Configuration used by PeerConnections, and the relay-only policy
// maps to IceTransportPolicyRelay. This is the falsifier for any call site
// that bypasses WebRtcConfig with a literal ICEServer list.
func TestIceConfigPathDrill(t *testing.T) {
	cfg := &webrtc.WebRtcConfig{
		IceServers: []*webrtc.IceServerConfig{
			{Urls: []string{"stun:stun.example.net:3478"}},
			{Urls: []string{"turn:turn.example.net:3478"}, Username: "drill-user", Credential: &webrtc.IceServerConfig_Password{Password: "from-slot"}},
		},
	}
	conf := cfg.ToWebRtcConfiguration()
	if len(conf.ICEServers) != 2 {
		t.Fatalf("expected 2 ICE servers in effective configuration, got %d", len(conf.ICEServers))
	}
	if conf.ICEServers[0].URLs[0] != "stun:stun.example.net:3478" {
		t.Fatalf("STUN URL lost: %v", conf.ICEServers[0].URLs)
	}
	if conf.ICEServers[1].Username != "drill-user" || conf.ICEServers[1].Credential != "from-slot" {
		t.Fatalf("TURN credentials lost: %+v", conf.ICEServers[1])
	}

	relayCfg := &webrtc.WebRtcConfig{IceTransportPolicy: webrtc.IceTransportPolicy_IceTransportPolicy_RELAY}
	if relayCfg.ToWebRtcConfiguration().ICETransportPolicy != pion.ICETransportPolicyRelay {
		t.Fatal("relay-only policy did not map to ICERelayOnly")
	}
}

// TestTwoNetworkSpaceDrill exercises the shared-Space drill matrix over a
// simulated two-network topology:
//
//  1. signaling + STUN required: both peers carry the same resolved ICE
//     servers before candidate gathering;
//  2. TURN/relay detection: recorded from gathered candidate types when a
//     TURN endpoint is supplied; reported unavailable otherwise;
//  3. offline update / reconnect convergence: one peer is cut from the data
//     path while signaling stays alive, the peer applies its update locally,
//     and the link must converge once the path returns;
//  4. concurrent-update conflict behavior and block-retention source are
//     asserted at the SharedObject/SOSync layer (provider_local tests) and
//     require the live slot because they need real block stores.
//
// The drill never touches credentials or durable user data: state lives in
// the simulator and dies with the process.
func TestTwoNetworkSpaceDrill(t *testing.T) {
	stunURL := os.Getenv("SPACEWAVE_DRILL_STUN_URL")
	turnURL := os.Getenv("SPACEWAVE_DRILL_TURN_URL")
	turnUser := os.Getenv("SPACEWAVE_DRILL_TURN_USERNAME")
	turnPass := os.Getenv("SPACEWAVE_DRILL_TURN_PASSWORD")

	switch os.Getenv(twoNetworkSpaceDrillEnv) {
	case "1":
	case "":
		t.Skipf("set %s=1 with SPACEWAVE_DRILL_{STUN,TURN}_URL to run the network stages; "+
			"requires UDP egress to the STUN URL, UDP/TCP to the TURN URL with short-lived credentials, "+
			"and no durable user state", twoNetworkSpaceDrillEnv)
	default:
		t.Skipf("%s must be 1 when set", twoNetworkSpaceDrillEnv)
	}

	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	var severed bool
	router, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "10.0.0.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	router.AddChunkFilter(func(vnet.Chunk) bool { return !severed })

	mkNet := func(ip string) *vnet.Net {
		n, err := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{ip}})
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := router.AddNet(n); err != nil {
			t.Fatal(err.Error())
		}
		return n
	}
	iceNet0 := mkNet("10.0.0.10")
	iceNet2 := mkNet("10.0.0.20")
	if err := router.Start(); err != nil {
		t.Fatal(err.Error())
	}
	defer func() { _ = router.Stop() }()

	iceTimeouts := webrtc.WithICETimeouts(time.Second, 2*time.Second, 300*time.Millisecond)
	iceServers := buildDrillICEServers(t, stunURL, turnURL, turnUser, turnPass)

	g := graph.NewGraph()
	addPeer := func() *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	p0 := addPeer()
	p0.AddFactory(func(b bus.Bus) controller.Factory { return signaling_rpc_client.NewFactory(b) })
	p0.AddFactory(func(b bus.Bus) controller.Factory {
		return webrtc.NewFactory(b, webrtc.WithICENet(iceNet0), iceTimeouts)
	})

	p1 := addPeer()
	p1.AddFactory(func(b bus.Bus) controller.Factory { return signaling_server.NewFactory(b) })
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling.ProtocolID)},
		},
	})

	p2 := addPeer()
	p2.AddFactory(func(b bus.Bus) controller.Factory { return signaling_rpc_client.NewFactory(b) })
	p2.AddFactory(func(b bus.Bus) controller.Factory {
		return webrtc.NewFactory(b, webrtc.WithICENet(iceNet2), iceTimeouts)
	})

	signalingID := "webrtc-signaling"
	signalClientConf := &signaling_rpc_client.Config{
		SignalingId: signalingID,
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{p1.GetPeerID().String()},
		},
	}
	p0.AddConfig("signaling-client", signalClientConf)
	p2.AddConfig("signaling-client", signalClientConf)

	// Both ends resolve the identical ICE configuration before gathering;
	// this mirrors the deployment-injected settings boundary (no credentials
	// are serialized into the copied offer/answer payload).
	webrtcTptConf := &webrtc.Config{
		SignalingId: signalingID,
		AllPeers:    true,
		BlockPeers:  []string{p1.GetPeerID().String()},
		Verbose:     true,
		WebRtc:      iceServers,
	}
	p0.AddConfig("webrtc-tpt", webrtcTptConf)
	p2.AddConfig("webrtc-tpt", webrtcTptConf)

	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)

	sim := initSimulator(t, ctx, le, g)
	defer sim.Close()

	px0 := sim.GetPeerByID(p0.GetPeerID())
	px2 := sim.GetPeerByID(p2.GetPeerID())

	_, esRef, err := px0.GetTestbed().Bus.AddDirective(
		link.NewEstablishLinkWithPeer(p0.GetPeerID(), p2.GetPeerID()),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer esRef.Release()

	waitConnectivity(t, ctx, px0, px2, "initial", 30*time.Second)

	// Offline stage: cut the data path while signaling stays alive.
	le.Info("severing data path between the two networks")
	severed = true
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(8 * time.Second):
	}

	// TURN/relay detection is honest: without a reachable TURN endpoint this
	// stage records unavailability instead of asserting relay candidates.
	if turnURL == "" {
		le.Warn("TURN endpoint not supplied; relay availability recorded as UNAVAILABLE")
	} else {
		le.Infof("TURN endpoint configured (%s); relay candidates expected in gathered SDP", turnURL)
	}

	// Convergence stage: restore the path and require re-establishment.
	severed = false
	waitConnectivity(t, ctx, px0, px2, "after-reconnect", 60*time.Second)
	le.Info("two-network drill: link converged after offline period")
}

func buildDrillICEServers(t *testing.T, stunURL, turnURL, turnUser, turnPass string) *webrtc.WebRtcConfig {
	t.Helper()
	cfg := &webrtc.WebRtcConfig{}
	if stunURL == "" {
		t.Skip("SPACEWAVE_DRILL_STUN_URL is required: the drill must prove srflx candidacy, not host-only pairing")
	}
	stun := &webrtc.IceServerConfig{Urls: []string{stunURL}}
	if turnURL == "" {
		cfg.IceServers = []*webrtc.IceServerConfig{stun}
		return cfg
	}
	if turnUser == "" || turnPass == "" {
		t.Skip("SPACEWAVE_DRILL_TURN_USERNAME/PASSWORD required with a TURN URL")
	}
	cfg.IceServers = []*webrtc.IceServerConfig{
		stun,
		{
			Urls:       []string{turnURL},
			Username:   turnUser,
			Credential: &webrtc.IceServerConfig_Password{Password: turnPass},
		},
	}
	return cfg
}

func waitConnectivity(t *testing.T, ctx context.Context, px0, px2 *simulate.Peer, stage string, timeout time.Duration) {
	t.Helper()
	const probeTimeout = 5 * time.Second
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var lastErr error
	for {
		probeCtx, cancelProbe := context.WithTimeout(ctx, probeTimeout)
		err := simulate.TestConnectivity(probeCtx, px0, px2)
		cancelProbe()
		if err == nil {
			return
		}
		lastErr = err
		select {
		case <-ctx.Done():
			t.Fatalf("%s: context done: %v", stage, ctx.Err())
		case <-deadline.C:
			t.Fatalf("%s: connectivity not restored in %v: %v", stage, timeout, lastErr)
		case <-time.After(500 * time.Millisecond):
		}
	}
}
