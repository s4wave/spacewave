//go:build !js

package provider_local_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pion/logging"
	"github.com/pion/transport/v4/vnet"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	signaling_rpc_client "github.com/s4wave/spacewave/net/signaling/rpc/client"
	signaling_server "github.com/s4wave/spacewave/net/signaling/rpc/server"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_server "github.com/s4wave/spacewave/net/stream/srpc/server"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	tc "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// drillSlotEnv gates every drill stage that starts network machinery.
const drillSlotEnv = "SPACEWAVE_TWO_NETWORK_DRILL"

// TestTwoNetworkSpaceDrillJoinedAccounts joins two disposable authenticated
// provider accounts onto separate simulated network paths and runs the
// shared-Space acceptance sequence:
//
//  1. both accounts negotiate their data path over WebRTC through a
//     disposable signaling-hub account; no direct bootstrap link exists
//     between them;
//  2. initial convergence: one shared state update replicates A -> B;
//  3. offline durable update: with the cross-network path severed, A
//     applies durable updates locally;
//  4. concurrent conflicting update: B applies its own conflicting update
//     during the same partition;
//  5. reconnect convergence: SOSync adopts the higher-seqno root and the
//     losing concurrent update is discarded;
//  6. retained block source: with every link severed again, B retains and
//     reads the converged state locally from its own stores.
//
// All state is disposable: each account lives in its own in-memory testbed
// and dies with the process.
//
// Topology honesty: the single-router vnet proves the direct cross-network
// class where host candidates are routable. Proving the symmetric-NAT /
// relay-required class needs a disposable TURN service, which the repository
// does not yet provide; relay availability stays UNAVAILABLE until a future
// slot supplies one.
func TestTwoNetworkSpaceDrillJoinedAccounts(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)
	if os.Getenv(drillSlotEnv) != "1" {
		t.Skipf("set %s=1 to run the joined two-network drill; it starts simulated "+
			"network paths and needs a supervised slot", drillSlotEnv)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)

	var severed atomic.Bool
	router, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "10.0.0.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	router.AddChunkFilter(func(vnet.Chunk) bool { return !severed.Load() })
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
	iceNetA := mkNet("10.0.0.10")
	iceNetB := mkNet("10.0.0.20")
	if err := router.Start(); err != nil {
		t.Fatal(err.Error())
	}
	defer func() { _ = router.Stop() }()

	// Three disposable authenticated accounts: peers A and B carry the shared
	// Space state; C is the signaling hub only.
	tbA, sessRefA, accA, sessA, releaseA := setupProviderAndSession(ctx, t)
	defer releaseA()
	tbB, sessRefB, accB, sessB, releaseB := setupProviderAndSession(ctx, t)
	defer releaseB()
	_, _, accC, sessC, releaseC := setupProviderAndSession(ctx, t)
	defer releaseC()

	if err := accA.CreateSessionTransport(ctx, sessA.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer accA.StopSessionTransport()
	if err := accB.CreateSessionTransport(ctx, sessB.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer accB.StopSessionTransport()
	if err := accC.CreateSessionTransport(ctx, sessC.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	defer accC.StopSessionTransport()

	accountIDA := sessRefA.GetProviderResourceRef().GetProviderAccountId()
	accountIDB := sessRefB.GetProviderResourceRef().GetProviderAccountId()

	childBusA := accA.GetSessionTransport().GetChildBus()
	childBusB := accB.GetSessionTransport().GetChildBus()
	childBusC := accC.GetSessionTransport().GetChildBus()

	peerIDA := accA.GetSessionTransport().GetPeerID()
	peerIDB := accB.GetSessionTransport().GetPeerID()
	peerIDC := accC.GetSessionTransport().GetPeerID()

	// Bootstrap links A<->C and B<->C over inproc carry only the signaling
	// streams; there is deliberately no A<->B inproc path, so the only route
	// the sync data can take is the negotiated WebRTC path.
	connectInprocPair(ctx, t, childBusA, peerIDA, childBusC, peerIDC)
	connectInprocPair(ctx, t, childBusB, peerIDB, childBusC, peerIDC)

	sigSrv, err := signaling_server.NewController(le, childBusC, &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds: []string{peerIDC.String()},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := childBusC.AddController(ctx, sigSrv, nil); err != nil {
		t.Fatal(err)
	}

	sigClientConf := &signaling_rpc_client.Config{
		SignalingId: "webrtc",
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{peerIDC.String()},
		},
	}
	for _, bc := range []bus.Bus{childBusA, childBusB} {
		sigCli, err := signaling_rpc_client.NewController(le, bc, sigClientConf)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bc.AddController(ctx, sigCli, nil); err != nil {
			t.Fatal(err)
		}
	}

	// WebRTC data transports, one per simulated network. The hub peer is
	// blocked so the only negotiable data path is A <-> B.
	mountWebrtcTransport(t, ctx, le, childBusA, iceNetA, peerIDC)
	mountWebrtcTransport(t, ctx, le, childBusB, iceNetB, peerIDC)

	addEstablishLink(ctx, t, childBusA, peerIDA, peerIDB)
	// Hold the reverse direction too: EstablishLinkWithPeer instances with
	// no retained reference are disposed after holdOpenDur (10s) and the
	// dispose callback closes the underlying link. Holding only A->B left
	// the B-side instance to idle out deterministically 10s after every
	// re-establishment.
	addEstablishLink(ctx, t, childBusB, peerIDB, peerIDA)

	// Stage 2: initial convergence of one shared update A -> B.
	soA, relSoA := mountAccountSettingsSO(ctx, t, tbA.Bus, accountIDA)
	defer relSoA()
	addPairedDeviceAndWait(ctx, t, soA, "12D3KooWDrillSeedPeer", "drill seed")

	if err := accA.StartP2PSync(ctx, accA.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	defer accA.StopP2PSync()
	if err := accB.StartP2PSync(ctx, accB.GetSessionTransport()); err != nil {
		t.Fatal(err)
	}
	defer accB.StopP2PSync()

	waitForSyncedRootSeqno(ctx, t, accB, account_settings.BindingPurpose, 1)

	// Stage 3+4: sever the cross-network data path; A commits durable
	// updates while offline; B commits its own conflicting update during
	// the same partition.
	le.Info("severing cross-network data path")
	severed.Store(true)
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(8 * time.Second):
	}

	addPairedDeviceAndWait(ctx, t, soA, "12D3KooWDrillOffline1", "offline update 1")
	addPairedDeviceAndWait(ctx, t, soA, "12D3KooWDrillOffline2", "offline update 2")

	soB, relSoB := mountAccountSettingsSO(ctx, t, tbB.Bus, accountIDB)
	addPairedDeviceAndWait(ctx, t, soB, "12D3KooWDrillConflict", "concurrent conflict")
	relSoB()

	// Stage 5: restore the path. The declared conflict rule is that SOSync
	// adopts the higher-seqno root wholesale; the losing concurrent update
	// must be discarded, not merged. The equal-seqno tie-break is an
	// untested existing product property: this drill keeps the winner
	// unambiguous (3 vs 2) and does not assert tie behavior.
	le.Info("restoring cross-network data path")
	severed.Store(false)
	waitForSyncedRootSeqno(ctx, t, accB, account_settings.BindingPurpose, 3)

	devicesB := readPairedDevices(ctx, t, tbB.Bus, accountIDB)
	if _, ok := devicesB["12D3KooWDrillConflict"]; ok {
		t.Fatal("losing concurrent update survived convergence; expected higher-seqno root to discard it")
	}
	for _, want := range []string{"12D3KooWDrillOffline1", "12D3KooWDrillOffline2"} {
		if _, ok := devicesB[want]; !ok {
			t.Fatalf("offline update %s missing from converged state on B", want)
		}
	}

	// Stage 6: retained block source. Every path out of B goes down again;
	// B must retain and read the converged state locally from its own
	// stores.
	le.Info("severing all paths to verify retained block source on B")
	severed.Store(true)
	waitForSyncedRootSeqno(ctx, t, accB, account_settings.BindingPurpose, 3)
	devicesB = readPairedDevices(ctx, t, tbB.Bus, accountIDB)
	if _, ok := devicesB["12D3KooWDrillOffline2"]; !ok {
		t.Fatal("B lost retained state while fully disconnected")
	}
	le.Info("retained block source: B retains and reads converged state locally while fully disconnected")
}

// connectInprocPair connects two session child buses over inproc as a
// bootstrap-only path: mutual dialers, a direct wire, and one held
// EstablishLink directive.
func connectInprocPair(ctx context.Context, t *testing.T, busX bus.Bus, peerX peer.ID, busY bus.Bus, peerY peer.ID) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())

	ctrlX := inproc.BuildInprocController(le, busX, "", &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			peerY.String(): {Address: inproc.NewAddr(peerY).String()},
		},
	})
	ctrlY := inproc.BuildInprocController(le, busY, "", &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			peerX.String(): {Address: inproc.NewAddr(peerX).String()},
		},
	})
	if _, err := busX.AddController(ctx, ctrlX, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := busY.AddController(ctx, ctrlY, nil); err != nil {
		t.Fatal(err)
	}

	tptX, err := ctrlX.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tptY, err := ctrlY.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ipX := tptX.(*inproc.Inproc)
	ipY := tptY.(*inproc.Inproc)
	ipX.ConnectToInproc(ctx, ipY)
	ipY.ConnectToInproc(ctx, ipX)

	// One held EstablishLink per pair: establishing from both sides causes
	// dual-dial instability (see connectSessionTransports).
	addEstablishLink(ctx, t, busX, peerX, peerY)
}

// mountWebrtcTransport adds a WebRTC transport controller bound to a
// simulated vnet to a session child bus, mirroring webrtc.Factory.Construct.
func mountWebrtcTransport(t *testing.T, ctx context.Context, le *logrus.Entry, b bus.Bus, iceNet *vnet.Net, blockPeer peer.ID) {
	t.Helper()

	conf := &webrtc.Config{
		SignalingId: "webrtc",
		AllPeers:    true,
		BlockPeers:  []string{blockPeer.String()},
		Verbose:     true,
	}
	rtc := tc.NewController(
		le,
		b,
		controller.NewInfo(webrtc.ControllerID, webrtc.Version, "two-network drill webrtc"),
		peer.ID(""),
		true,
		func(ctx context.Context, le *logrus.Entry, pkey crypto.PrivKey, handler transport.TransportHandler) (transport.Transport, error) {
			return webrtc.NewWebRTC(
				ctx,
				le,
				b,
				conf,
				pkey,
				handler,
				webrtc.WithICENet(iceNet),
				// Short ICE timeouts so a severed path reaches "failed"
				// quickly and the session restarts on restore.
				webrtc.WithICETimeouts(time.Second, 2*time.Second, 300*time.Millisecond),
			)
		},
	)
	if _, err := b.AddController(ctx, rtc, nil); err != nil {
		t.Fatal(err)
	}
}

// readPairedDevices mounts the account settings SO and returns its current
// paired devices as peer ID -> display name.
func readPairedDevices(ctx context.Context, t *testing.T, b bus.Bus, accountID string) map[string]string {
	t.Helper()

	so, relSO := mountAccountSettingsSO(ctx, t, b, accountID)
	defer relSO()

	stateCtr, relStateCtr, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relStateCtr()

	rootInner, err := stateCtr.GetValue().GetRootInner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings := &account_settings.AccountSettings{}
	if data := rootInner.GetStateData(); len(data) > 0 {
		if err := settings.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
	}
	out := make(map[string]string)
	for _, d := range settings.GetPairedDevices() {
		out[d.GetPeerId()] = d.GetDisplayName()
	}
	return out
}

// peerLinkDrill is the common two-account WebRTC testbed for link
// retention tests: two accounts on separate simulated networks, a signaling
// hub account, and retained transport controller handles for observation.
type peerLinkDrill struct {
	le     *logrus.Entry
	logger *logrus.Logger
	ctrlA  *tc.Controller
	ctrlB  *tc.Controller
	accA   *provider_local.ProviderAccount
	accB   *provider_local.ProviderAccount
	soA    sobject.SharedObject
	soB    sobject.SharedObject
	soARel func()
	soBRel func()
	tbA    *testbed.Testbed
	tbB    *testbed.Testbed
}

// startPeerLinkDrill builds the testbed. When seedPaired is true both
// accounts get a paired-device record naming the other, exercising the
// owner-signed paired-device authority path.
func startPeerLinkDrill(ctx context.Context, t *testing.T, seedPaired bool) (*peerLinkDrill, func()) {
	t.Helper()

	logger := logrus.New()
	le := logrus.NewEntry(logger)

	router, err := vnet.NewRouter(&vnet.RouterConfig{
		CIDR:          "10.2.0.0/24",
		LoggerFactory: logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		t.Fatal(err.Error())
	}
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
	iceNetA := mkNet("10.2.0.10")
	iceNetB := mkNet("10.2.0.20")
	if err := router.Start(); err != nil {
		t.Fatal(err.Error())
	}

	tbA, sessRefA, accA, sessA, releaseA := setupProviderAndSession(ctx, t)
	tbB, sessRefB, accB, sessB, releaseB := setupProviderAndSession(ctx, t)
	_, _, accC, sessC, releaseC := setupProviderAndSession(ctx, t)

	release := func() {
		releaseA()
		releaseB()
		releaseC()
		_ = router.Stop()
	}

	if err := accA.CreateSessionTransport(ctx, sessA.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	if err := accB.CreateSessionTransport(ctx, sessB.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}
	if err := accC.CreateSessionTransport(ctx, sessC.GetPrivKey(), ""); err != nil {
		t.Fatal(err)
	}

	childBusA := accA.GetSessionTransport().GetChildBus()
	childBusB := accB.GetSessionTransport().GetChildBus()
	childBusC := accC.GetSessionTransport().GetChildBus()

	peerIDA := accA.GetSessionTransport().GetPeerID()
	peerIDB := accB.GetSessionTransport().GetPeerID()
	peerIDC := accC.GetSessionTransport().GetPeerID()

	connectInprocPair(ctx, t, childBusA, peerIDA, childBusC, peerIDC)
	connectInprocPair(ctx, t, childBusB, peerIDB, childBusC, peerIDC)

	sigSrv, err := signaling_server.NewController(le, childBusC, &signaling_server.Config{
		Server: &stream_srpc_server.Config{
			PeerIds: []string{peerIDC.String()},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := childBusC.AddController(ctx, sigSrv, nil); err != nil {
		t.Fatal(err)
	}
	sigClientConf := &signaling_rpc_client.Config{
		SignalingId: "webrtc",
		Client: &stream_srpc_client.Config{
			ServerPeerIds: []string{peerIDC.String()},
		},
	}
	for _, bc := range []bus.Bus{childBusA, childBusB} {
		sigCli, err := signaling_rpc_client.NewController(le, bc, sigClientConf)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bc.AddController(ctx, sigCli, nil); err != nil {
			t.Fatal(err)
		}
	}

	conf := &webrtc.Config{
		SignalingId: "webrtc",
		AllPeers:    true,
		BlockPeers:  []string{peerIDC.String()},
		Verbose:     true,
	}
	iceTimeouts := webrtc.WithICETimeouts(time.Second, 2*time.Second, 300*time.Millisecond)
	ctrlA := tc.NewController(
		le,
		childBusA,
		controller.NewInfo(webrtc.ControllerID, webrtc.Version, "peer-link drill webrtc A"),
		peer.ID(""),
		true,
		func(ctx context.Context, le *logrus.Entry, pkey crypto.PrivKey, handler transport.TransportHandler) (transport.Transport, error) {
			return webrtc.NewWebRTC(ctx, le, childBusA, conf, pkey, handler,
				webrtc.WithICENet(iceNetA), iceTimeouts)
		},
	)
	if _, err := childBusA.AddController(ctx, ctrlA, nil); err != nil {
		t.Fatal(err)
	}
	ctrlB := tc.NewController(
		le,
		childBusB,
		controller.NewInfo(webrtc.ControllerID, webrtc.Version, "peer-link drill webrtc B"),
		peer.ID(""),
		true,
		func(ctx context.Context, le *logrus.Entry, pkey crypto.PrivKey, handler transport.TransportHandler) (transport.Transport, error) {
			return webrtc.NewWebRTC(ctx, le, childBusB, conf, pkey, handler,
				webrtc.WithICENet(iceNetB), iceTimeouts)
		},
	)
	if _, err := childBusB.AddController(ctx, ctrlB, nil); err != nil {
		t.Fatal(err)
	}

	accountIDA := sessRefA.GetProviderResourceRef().GetProviderAccountId()
	accountIDB := sessRefB.GetProviderResourceRef().GetProviderAccountId()
	soA, soARel := mountAccountSettingsSO(ctx, t, tbA.Bus, accountIDA)
	soB, soBRel := mountAccountSettingsSO(ctx, t, tbB.Bus, accountIDB)
	if seedPaired {
		addPairedDeviceAndWait(ctx, t, soA, peerIDB.String(), "retained peer")
		addPairedDeviceAndWait(ctx, t, soB, peerIDA.String(), "retained peer")
	}

	d := &peerLinkDrill{
		le: le, logger: logger,
		ctrlA: ctrlA, ctrlB: ctrlB,
		accA: accA, accB: accB,
		soA: soA, soB: soB, soARel: soARel, soBRel: soBRel,
		tbA: tbA, tbB: tbB,
	}
	return d, func() {
		soARel()
		soBRel()
		release()
	}
}

// startSync starts P2P sync on both accounts concurrently so the test does
// not rely on startup ordering for symmetric dial behavior.
func (d *peerLinkDrill) startSync(ctx context.Context, t *testing.T) {
	var wg sync.WaitGroup
	var errA, errB error
	wg.Add(2)
	go func() { defer wg.Done(); errA = d.accA.StartP2PSync(ctx, d.accA.GetSessionTransport()) }()
	go func() { defer wg.Done(); errB = d.accB.StartP2PSync(ctx, d.accB.GetSessionTransport()) }()
	wg.Wait()
	if errA != nil {
		t.Fatal(errA)
	}
	if errB != nil {
		t.Fatal(errB)
	}
	t.Cleanup(func() {
		d.accA.StopP2PSync()
		d.accB.StopP2PSync()
	})
}

// waitForLinkedPeer blocks until the A-side transport reports a live link
// to want.
func (d *peerLinkDrill) waitForLinkedPeer(t *testing.T, want peer.ID, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		linked, _ := d.ctrlA.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{want})
		if _, ok := linked[want]; ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("link to %v not established within %v", want, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// linksTo counts live links on the A-side transport toward want.
func (d *peerLinkDrill) linksTo(want peer.ID) int {
	snaps, _ := d.ctrlA.GetLinkSnapshotsWithWait()
	n := 0
	for _, s := range snaps {
		if s.RemotePeerID == want {
			n++
		}
	}
	return n
}

// TestP2PSyncRetainsPeerLinksBeyondHoldOpen verifies that P2P sync retains
// an EstablishLinkWithPeer reference per locally authorized paired device,
// keeping the negotiated WebRTC link open past the link hold-open duration.
// With no retained reference the link is disposed and closed shortly after
// establishment, dropping the only data path between two syncing peers.
func TestP2PSyncRetainsPeerLinksBeyondHoldOpen(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	d, release := startPeerLinkDrill(ctx, t, true)
	defer release()
	peerIDB := d.accB.GetSessionTransport().GetPeerID()

	d.startSync(ctx, t)
	d.waitForLinkedPeer(t, peerIDB, 60*time.Second)

	// Hold past the link hold-open duration: a link with no retained
	// reference is disposed and closed within it.
	time.Sleep(12 * time.Second)
	linked, _ := d.ctrlA.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{peerIDB})
	if _, ok := linked[peerIDB]; !ok {
		t.Fatal("link to paired sync peer was closed by hold-open disposal; expected P2P sync to retain it")
	}
}

// TestP2PSyncCrossDialConvergesSymmetricRetention proves that symmetric
// retained directives converge: both sides dial at once, exactly one link
// per remote results, and it stays singular and stable past the hold-open
// window. Startup ordering is deliberately not relied upon.
func TestP2PSyncCrossDialConvergesSymmetricRetention(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	d, release := startPeerLinkDrill(ctx, t, true)
	defer release()
	peerIDB := d.accB.GetSessionTransport().GetPeerID()
	peerIDA := d.accA.GetSessionTransport().GetPeerID()

	d.startSync(ctx, t)
	d.waitForLinkedPeer(t, peerIDB, 60*time.Second)

	assertSingleStableLink := func(side string, ctrl *tc.Controller, want peer.ID) {
		t.Helper()
		if n := d.linksToOn(ctrl, want); n != 1 {
			t.Fatalf("%s: expected exactly 1 link to %v under symmetric dial, found %d", side, want, n)
		}
	}
	assertSingleStableLink("A", d.ctrlA, peerIDB)
	assertSingleStableLink("B", d.ctrlB, peerIDA)

	time.Sleep(12 * time.Second)
	assertSingleStableLink("A", d.ctrlA, peerIDB)
	assertSingleStableLink("B", d.ctrlB, peerIDA)
	linked, _ := d.ctrlA.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{peerIDB})
	if _, ok := linked[peerIDB]; !ok {
		t.Fatal("symmetric-dial link lost past hold-open window")
	}
}

// linksToOn counts live links on the given transport toward want.
func (d *peerLinkDrill) linksToOn(ctrl *tc.Controller, want peer.ID) int {
	snaps, _ := ctrl.GetLinkSnapshotsWithWait()
	n := 0
	for _, s := range snaps {
		if s.RemotePeerID == want {
			n++
		}
	}
	return n
}

// TestP2PSyncUnlinkReleasesRetainedLink verifies event-driven removal:
// unlinking a paired device releases the retained directive on both ends,
// and the link closes after the hold-open grace instead of persisting.
func TestP2PSyncUnlinkReleasesRetainedLink(t *testing.T) {
	skipFullP2PSyncUnderGoScript(t)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	d, release := startPeerLinkDrill(ctx, t, true)
	defer release()
	peerIDB := d.accB.GetSessionTransport().GetPeerID()
	peerIDA := d.accA.GetSessionTransport().GetPeerID()

	d.startSync(ctx, t)
	d.waitForLinkedPeer(t, peerIDB, 60*time.Second)

	removePairedDevice(ctx, t, d.soA, peerIDB.String())
	removePairedDevice(ctx, t, d.soB, peerIDA.String())

	// Release drops the references immediately; the dispose timer closes
	// the idle link within the hold-open duration.
	deadline := time.Now().Add(30 * time.Second)
	for {
		linked, _ := d.ctrlA.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{peerIDB})
		if _, ok := linked[peerIDB]; !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("link to unlinked peer survived past the hold-open grace; expected reconcile to release it")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// removePairedDevice queues a RemovePairedDevice op on the settings object.
func removePairedDevice(ctx context.Context, t *testing.T, so sobject.SharedObject, peerID string) {
	t.Helper()
	op := &account_settings.AccountSettingsOp{
		Op: &account_settings.AccountSettingsOp_RemovePairedDevice{
			RemovePairedDevice: &account_settings.RemovePairedDeviceOp{
				PeerId: peerID,
			},
		},
	}
	opData, err := op.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := so.QueueOperation(ctx, opData); err != nil {
		t.Fatal(err)
	}
}
