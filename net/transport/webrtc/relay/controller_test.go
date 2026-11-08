//go:build !js

package webrtc_relay_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pion/turn/v5"
	"github.com/s4wave/spacewave/net/peer"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/s4wave/spacewave/net/sim/tests"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	webrtc_relay "github.com/s4wave/spacewave/net/transport/webrtc/relay"
	"github.com/sirupsen/logrus"
)

// testIssuer issues one-use TURN users that its TURN server accepts.
type testIssuer struct {
	// addr is the TURN server's address.
	addr string
	// users maps each issued username to its password.
	users sync.Map
	// issued counts issued credentials.
	issued atomic.Int32
	// auths counts TURN authentication attempts.
	auths atomic.Int32
}

// Issue issues a fresh TURN user for the session, valid for six hours.
func (s *testIssuer) Issue(_ context.Context, session peer.ID) (*webrtc_relay.Credentials, error) {
	// Mint a user unique to this issue.
	n := s.issued.Add(1)
	user := session.String() + "-" + strconv.Itoa(int(n))
	pass := make([]byte, 16)
	_, _ = rand.Read(pass)
	password := hex.EncodeToString(pass)
	s.users.Store(user, password)

	// Name the relay alongside a STUN URL and an uncredentialed server the
	// client must drop.
	return &webrtc_relay.Credentials{
		IceServers: []*webrtc.IceServerConfig{
			{Urls: []string{"stun:" + s.addr}},
			{
				Urls:       []string{"stun:" + s.addr, "turn:" + s.addr + "?transport=udp"},
				Username:   user,
				Credential: &webrtc.IceServerConfig_Password{Password: password},
			},
		},
		ExpiresAt: timestamppb.New(time.Now().Add(6 * time.Hour)),
	}, nil
}

// passwords returns every issued password.
func (s *testIssuer) passwords() []string {
	var out []string
	s.users.Range(func(_, v any) bool {
		out = append(out, v.(string))
		return true
	})
	return out
}

// startTURN runs a TURN server on loopback that accepts the users issuer
// issued, and points issuer at it.
func startTURN(t *testing.T, issuer *testIssuer) {
	t.Helper()
	conn, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: "spacewave.test",
		AuthHandler: func(ra *turn.RequestAttributes) (string, []byte, bool) {
			issuer.auths.Add(1)
			pass, ok := issuer.users.Load(ra.Username)
			if !ok {
				return "", nil, false
			}
			return ra.Username, turn.GenerateAuthKey(ra.Username, ra.Realm, pass.(string)), true
		},
		PacketConnConfigs: []turn.PacketConnConfig{{
			PacketConn:            conn,
			RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{RelayAddress: net.ParseIP("127.0.0.1"), Address: "127.0.0.1"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	issuer.addr = conn.LocalAddr().String()
}

// TestSessionRelaysLink connects two peers whose transports accept only relay
// candidates through a local TURN server, with credentials each session
// fetches from the issuer, and checks no credential reaches the logs or the
// directive debug values.
func TestSessionRelaysLink(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// Capture every log line the peers write.
	var logs bytes.Buffer
	var logsMtx sync.Mutex
	log := logrus.New()
	log.SetLevel(logrus.TraceLevel)
	log.SetOutput(writerFunc(func(p []byte) (int, error) {
		logsMtx.Lock()
		defer logsMtx.Unlock()
		return logs.Write(p)
	}))
	le := logrus.NewEntry(log)

	// Run the TURN server the issuer's credentials open.
	issuer := &testIssuer{}
	startTURN(t, issuer)

	// Signal through p1; p0 and p2 link only through relay candidates.
	g := graph.NewGraph()
	addPeer := func() *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p0, p1, p2 := addPeer(), addPeer(), addPeer()
	p1.AddFactory(func(b bus.Bus) controller.Factory { return signaling_server.NewFactory(b) })
	p1.AddConfig("signaling-server", &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds:     []string{p1.GetPeerID().String()},
			ProtocolIds: []string{string(signaling.ProtocolID)},
		},
	})
	relayOnly := &webrtc.WebRtcConfig{IceTransportPolicy: webrtc.IceTransportPolicy_IceTransportPolicy_RELAY}
	var fetches atomic.Int32
	for _, p := range []*graph.Peer{p0, p2} {
		session := p.GetPeerID()
		p.AddFactory(func(b bus.Bus) controller.Factory { return signaling_rpc_client.NewFactory(b) })
		p.AddFactory(func(b bus.Bus) controller.Factory { return webrtc.NewFactory(b) })
		p.AddFactoryAdder(func(b bus.Bus, _ *static.Resolver) {
			// The session's client fetches from the app server, here the issuer.
			fetch := func(ctx context.Context) (*webrtc_relay.Credentials, error) {
				fetches.Add(1)
				return issuer.Issue(ctx, session)
			}
			rel, err := b.AddController(ctx, webrtc_relay.NewController(session, fetch, relayOnly), nil)
			if err != nil {
				t.Error(err)
				return
			}
			t.Cleanup(rel)
		})
		p.AddConfig("signaling-client", &signaling_rpc_client.Config{
			SignalingId: "webrtc-signaling",
			Client:      &stream_srpc_client.Config{ServerPeerIds: []string{p1.GetPeerID().String()}},
		})
		p.AddConfig("webrtc-tpt", &webrtc.Config{
			SignalingId: "webrtc-signaling",
			AllPeers:    true,
			BlockPeers:  []string{p1.GetPeerID().String()},
			Verbose:     true,
			WebRtc:      relayOnly,
		})
	}
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)

	// The peers link through the relay.
	sim := tests.InitSimulator(t, ctx, le, g)
	if err := simulate.TestConnectivity(ctx, sim.GetPeerByID(p0.GetPeerID()), sim.GetPeerByID(p2.GetPeerID())); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() == 0 || issuer.auths.Load() == 0 {
		t.Fatalf("relay unused: %d fetches, %d TURN authentications", fetches.Load(), issuer.auths.Load())
	}

	// No issued password appears in a log line or a directive's debug values.
	passwords := issuer.passwords()
	logsMtx.Lock()
	written := logs.String()
	logsMtx.Unlock()
	for _, p := range []*graph.Peer{p0, p1, p2} {
		for _, di := range sim.GetPeerByID(p.GetPeerID()).GetTestbed().Bus.GetDirectives() {
			info, err := directive.NewDirectiveInfo(di.GetDirective()).MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			written += "\n" + string(info)
		}
	}
	for _, pass := range passwords {
		if strings.Contains(written, pass) {
			t.Fatal("an issued relay password was logged or shown in directive debug values")
		}
	}
}

// writerFunc adapts a function to io.Writer.
type writerFunc func([]byte) (int, error)

// Write calls f.
func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// TestSessionLookupScope checks the controller answers only its own session,
// refuses credentials about to expire after fetching once more, and keeps the
// base config's servers and policy.
func TestSessionLookupScope(t *testing.T) {
	ctx := t.Context()
	sessions := make([]peer.ID, 2)
	for i := range sessions {
		p, err := peer.NewPeer(nil)
		if err != nil {
			t.Fatal(err)
		}
		sessions[i] = p.GetPeerID()
	}

	// Each fetch returns the next expiry.
	var expiries []time.Duration
	var fetched int
	fetch := func(context.Context) (*webrtc_relay.Credentials, error) {
		exp := expiries[fetched]
		fetched++
		return &webrtc_relay.Credentials{
			IceServers: []*webrtc.IceServerConfig{
				{Urls: []string{"turn:open.test:3478"}},
				{
					Urls:       []string{"stun:relay.test:3478", "turns:relay.test:443?transport=tcp"},
					Username:   "u",
					Credential: &webrtc.IceServerConfig_Password{Password: "p"},
				},
			},
			ExpiresAt: timestamppb.New(time.Now().Add(exp)),
		}, nil
	}
	base := &webrtc.WebRtcConfig{
		IceServers:         []*webrtc.IceServerConfig{{Urls: []string{"stun:stun.test:3478"}}},
		IceTransportPolicy: webrtc.IceTransportPolicy_IceTransportPolicy_RELAY,
	}
	ctrl := webrtc_relay.NewController(sessions[0], fetch, base)
	b, rel := newBus(t, ctrl)
	defer rel()

	// Another peer on the same bus resolves nothing.
	if prov, release, err := webrtc.ExLookupICEProvider(ctx, b, sessions[1]); err != nil || prov != nil {
		if release != nil {
			release()
		}
		t.Fatalf("other session resolved a provider: %v", err)
	}
	prov, release, err := webrtc.ExLookupICEProvider(ctx, b, sessions[0])
	if err != nil || prov == nil {
		t.Fatalf("session provider: %v", err)
	}
	defer release()

	// A credential expiring within a minute is refused and fetched again.
	expiries, fetched = []time.Duration{30 * time.Second, 6 * time.Hour}, 0
	conf, err := prov.ICEConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fetched != 2 || len(conf.GetIceServers()) != 2 || conf.GetIceTransportPolicy() != base.GetIceTransportPolicy() {
		t.Fatalf("fetched %d, config %v", fetched, conf)
	}
	if urls := conf.GetIceServers()[1].GetUrls(); len(urls) != 1 || urls[0] != "turns:relay.test:443?transport=tcp" {
		t.Fatalf("relay urls %v", urls)
	}
	if len(base.GetIceServers()) != 1 {
		t.Fatal("the base config was modified")
	}

	// Two expiring fetches start no link.
	expiries, fetched = []time.Duration{30 * time.Second, 10 * time.Second}, 0
	if _, err := prov.ICEConfig(ctx); err != webrtc_relay.ErrExpiring {
		t.Fatalf("expiring credentials: %v", err)
	}
}

// newBus starts a bus running ctrl.
func newBus(t *testing.T, ctrl controller.Controller) (bus.Bus, func()) {
	t.Helper()
	b, _, err := core.NewCoreBus(t.Context(), logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	rel, err := b.AddController(t.Context(), ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	return b, rel
}
