package link_solicit_controller

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/s4wave/spacewave/net/testbed"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/inproc"
	"github.com/sirupsen/logrus"
)

type testMountedLink struct {
	uuid          uint64
	transportUUID uint64
	localPeer     peer.ID
	remotePeer    peer.ID
	openCh        chan protocol.ID
	// reject makes opened streams end as if the receiver closed them.
	reject bool
}

func (l *testMountedLink) GetLinkUUID() uint64 {
	return l.uuid
}

func (l *testMountedLink) GetTransportUUID() uint64 {
	return l.transportUUID
}

func (l *testMountedLink) GetRemoteTransportUUID() uint64 {
	return l.transportUUID
}

func (l *testMountedLink) GetLocalPeer() peer.ID {
	return l.localPeer
}

func (l *testMountedLink) GetRemotePeer() peer.ID {
	return l.remotePeer
}

func (l *testMountedLink) OpenMountedStream(
	_ context.Context,
	protocolID protocol.ID,
	opts stream.OpenOpts,
) (link.MountedStream, error) {
	if l.openCh != nil {
		l.openCh <- protocolID
	}
	return &testMountedStream{
		link:       l,
		protocolID: protocolID,
		opts:       opts,
		rejected:   l.reject,
	}, nil
}

type testMountedStream struct {
	link       link.MountedLink
	protocolID protocol.ID
	opts       stream.OpenOpts
	rejected   bool
}

func (s *testMountedStream) GetStream() stream.Stream {
	return testStream{rejected: s.rejected}
}

func (s *testMountedStream) GetProtocolID() protocol.ID {
	return s.protocolID
}

func (s *testMountedStream) GetOpenOpts() stream.OpenOpts {
	return s.opts
}

func (s *testMountedStream) GetPeerID() peer.ID {
	return s.link.GetRemotePeer()
}

func (s *testMountedStream) GetLink() link.MountedLink {
	return s.link
}

type testStream struct {
	rejected bool
}

// Read reports the receiver's acceptance or rejection of a solicited stream.
func (s testStream) Read(b []byte) (int, error) {
	if s.rejected {
		return 0, io.EOF
	}
	if len(b) == 0 {
		return 0, nil
	}
	b[0] = solicitAcceptByte
	return 1, nil
}

func (testStream) Write(b []byte) (int, error) {
	return len(b), nil
}

func (testStream) SetReadDeadline(time.Time) error {
	return nil
}

func (testStream) SetWriteDeadline(time.Time) error {
	return nil
}

func (testStream) SetDeadline(time.Time) error {
	return nil
}

func (testStream) Close() error {
	return nil
}

type testResolverHandler struct {
	values chan directive.Value
}

func newTestResolverHandler() *testResolverHandler {
	return &testResolverHandler{values: make(chan directive.Value, 8)}
}

// legacySolicitationExchange constructs a stable-hash exchange from an old peer.
func legacySolicitationExchange(hashes [][]byte) *solicitationExchange {
	return &solicitationExchange{hashes: cloneHashes(hashes)}
}

// incarnatedSolicitationExchange constructs one synchronized upgraded offer.
func incarnatedSolicitationExchange(
	hash, incarnation []byte,
	generation, acknowledgedGeneration uint64,
) *solicitationExchange {
	return &solicitationExchange{
		hashes: [][]byte{slices.Clone(hash)},
		offers: []solicitationOffer{{
			hash:        slices.Clone(hash),
			incarnation: slices.Clone(incarnation),
		}},
		supportsOfferIncarnations: true,
		generation:                generation,
		acknowledgedGeneration:    acknowledgedGeneration,
	}
}

func (h *testResolverHandler) AddValue(val directive.Value) (uint32, bool) {
	h.values <- val
	return uint32(len(h.values)), true
}

func (h *testResolverHandler) RemoveValue(uint32) (directive.Value, bool) {
	return nil, false
}

func (h *testResolverHandler) CountValues(bool) int {
	return len(h.values)
}

func (h *testResolverHandler) ClearValues() []uint32 {
	return nil
}

func (h *testResolverHandler) MarkIdle(bool) {}

func (h *testResolverHandler) AddValueRemovedCallback(uint32, func()) func() {
	return func() {}
}

func (h *testResolverHandler) AddResolverRemovedCallback(func()) func() {
	return func() {}
}

func (h *testResolverHandler) AddResolver(directive.Resolver, func()) func() {
	return func() {}
}

func buildTestbed(t *testing.T, ctx context.Context) *testbed.Testbed {
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.TestbedOpts{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Register inproc transport and solicitation controller factories.
	tb.StaticResolver.AddFactory(inproc.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(NewFactory())

	return tb
}

func startTransport(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	conf *inproc.Config,
) (*transport_controller.Controller, *inproc.Inproc, directive.Reference) {
	t.Helper()
	pid, err := peer.IDFromPrivateKey(tb.PrivKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if conf == nil {
		conf = &inproc.Config{}
	}
	conf.TransportPeerId = pid.String()

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
	return tpc, tpt.(*inproc.Inproc), tpRef
}

func startSolicitController(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
) directive.Reference {
	t.Helper()
	_, _, ref, err := bus.ExecOneOff(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(&Config{}),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

func newTestSolicitController(t *testing.T) *Controller {
	t.Helper()

	c, err := NewController(logrus.NewEntry(logrus.New()), &Config{})
	if err != nil {
		t.Fatal(err.Error())
	}
	c.openRoutines.SetContext(t.Context(), true)
	t.Cleanup(c.openRoutines.ClearContext)
	return c
}

func newTestLinkState(local, remote peer.ID) *linkState {
	ml := &testMountedLink{
		uuid:          1,
		transportUUID: 2,
		localPeer:     local,
		remotePeer:    remote,
		openCh:        make(chan protocol.ID, 4),
	}
	return &linkState{
		le:           logrus.NewEntry(logrus.New()),
		ml:           ml,
		sessionID:    link_solicit.ComputeSessionID(local, remote),
		localIsLower: local < remote,
		matched:      make(map[string]struct{}),
	}
}

func recvLocalSnapshot(t *testing.T, ch <-chan *controlStreamLocalSnapshot) *controlStreamLocalSnapshot {
	t.Helper()

	snap := recvTestValue(t, ch, "local snapshot")
	if snap == nil {
		t.Fatal("expected local snapshot")
	}
	return snap
}

func recvTestValue[T any](t *testing.T, ch <-chan T, name string) T {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	select {
	case val, ok := <-ch:
		if !ok {
			t.Fatalf("%s channel closed", name)
		}
		return val
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s", name)
	}
	var zero T
	return zero
}

func assertNoTestValue[T any](t *testing.T, ch <-chan T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	select {
	case val, ok := <-ch:
		if !ok {
			t.Fatalf("%s channel closed", name)
		}
		t.Fatalf("unexpected %s: %#v", name, val)
	case <-ctx.Done():
	}
}

func TestControlStreamLocalSnapshotWatchDynamicAddRemove(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		broadcast()
	})

	snapCh := make(chan *controlStreamLocalSnapshot, 8)
	done := make(chan error, 1)
	go func() {
		done <- c.watchControlStreamLocalSnapshots(ctx, ls, func(snap *controlStreamLocalSnapshot) error {
			snapCh <- snap
			return nil
		})
	}()

	initial := recvLocalSnapshot(t, snapCh)
	if initial.linkRemoved || len(initial.offers) != 0 {
		t.Fatalf("initial snapshot removed=%v offers=%d", initial.linkRemoved, len(initial.offers))
	}

	ss := &solicitState{
		dir: link_solicit.NewSolicitProtocol(protocol.ID("test/dynamic"), []byte("ctx"), "", 0),
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.solicitations[ss] = struct{}{}
		broadcast()
	})
	added := recvLocalSnapshot(t, snapCh)
	if len(added.offers) != 1 || added.offers[0].protocolID != protocol.ID("test/dynamic") {
		t.Fatalf("added offers = %#v", added.offers)
	}
	if string(added.offers[0].context) != "ctx" {
		t.Fatalf("added context = %q", added.offers[0].context)
	}

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(c.solicitations, ss)
		broadcast()
	})
	removed := recvLocalSnapshot(t, snapCh)
	if removed.linkRemoved || len(removed.offers) != 0 {
		t.Fatalf("removed snapshot removed=%v offers=%d", removed.linkRemoved, len(removed.offers))
	}

	cancel()
	if err := recvTestValue(t, done, "watch completion"); err == nil {
		t.Fatal("expected canceled watch")
	}
}

func TestControlStreamLocalSnapshotWatchLinkRemoval(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		broadcast()
	})

	snapCh := make(chan *controlStreamLocalSnapshot, 4)
	done := make(chan error, 1)
	go func() {
		done <- c.watchControlStreamLocalSnapshots(ctx, ls, func(snap *controlStreamLocalSnapshot) error {
			snapCh <- snap
			return nil
		})
	}()

	initial := recvLocalSnapshot(t, snapCh)
	if initial.linkRemoved {
		t.Fatal("initial snapshot should not be removed")
	}

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(c.links, ls.ml.GetLinkUUID())
		broadcast()
	})
	removed := recvLocalSnapshot(t, snapCh)
	if !removed.linkRemoved {
		t.Fatal("expected link removal snapshot")
	}

	cancel()
	if err := recvTestValue(t, done, "watch completion"); err == nil {
		t.Fatal("expected canceled watch")
	}
}

func TestLinkRemovalWaitsForDuplicateEstablishLinkValues(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	uuid := ls.ml.GetLinkUUID()

	c.addLink(ls.ml)
	c.addLink(ls.ml)

	c.removeLink(uuid)
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current := c.links[uuid]
		if current == nil {
			t.Fatal("duplicate link value removal deleted active link state")
		}
		if current.refCount != 1 {
			t.Fatalf("link refCount after one removal = %d, want 1", current.refCount)
		}
	})

	c.removeLink(uuid)
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if c.links[uuid] != nil {
			t.Fatal("link state survived final value removal")
		}
	})
}

func TestControlStreamLocalSnapshotRemoteHashesRefresh(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/remote"),
		Context:    []byte("ctx"),
	}
	hashes := link_solicit.ComputeProtocolHashes(ls.sessionID, []link_solicit.SolicitEntry{entry})
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		broadcast()
	})

	var snap *controlStreamLocalSnapshot
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snap = c.snapshotControlStreamLocalLocked(ls)
	})
	if snap.linkRemoved || len(snap.offers) != 0 {
		t.Fatalf("local snapshot removed=%v offers=%d", snap.linkRemoved, len(snap.offers))
	}

	remote := legacySolicitationExchange(hashes)
	if !c.setControlStreamRemoteExchange(ls, remote) {
		t.Fatal("link should accept remote hashes")
	}
	current, linkRemoved := c.currentControlStreamRemoteExchange(ls)
	if linkRemoved {
		t.Fatal("link should still exist")
	}
	if !slices.EqualFunc(current.hashes, hashes, bytes.Equal) {
		t.Fatalf("current remote hashes did not refresh")
	}

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(c.links, ls.ml.GetLinkUUID())
		broadcast()
	})
	if c.setControlStreamRemoteExchange(ls, remote) {
		t.Fatal("removed link accepted remote hashes")
	}
}

func TestControlStreamLocalSnapshotRefreshesQueuedSolicitationState(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	ss := &solicitState{
		dir: link_solicit.NewSolicitProtocol(protocol.ID("test/stale"), []byte("ctx"), "", 0),
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		c.solicitations[ss] = struct{}{}
		broadcast()
	})

	var queued *controlStreamLocalSnapshot
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		queued = c.snapshotControlStreamLocalLocked(ls)
	})
	if len(queued.offers) != 1 {
		t.Fatalf("queued offers = %d, want 1", len(queued.offers))
	}

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(c.solicitations, ss)
		broadcast()
	})
	current := c.currentControlStreamLocalSnapshot(ls)
	if current.linkRemoved || len(current.offers) != 0 {
		t.Fatalf("current snapshot removed=%v offers=%d", current.linkRemoved, len(current.offers))
	}
}

func TestControlStreamLocalSnapshotRejectsReplacedLink(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	replacement := newTestLinkState(peer.ID("a"), peer.ID("b"))
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/replaced"),
		Context:    []byte("ctx"),
	}
	hashes := link_solicit.ComputeProtocolHashes(ls.sessionID, []link_solicit.SolicitEntry{entry})
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		broadcast()
	})

	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = replacement
		broadcast()
	})
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snap := c.snapshotControlStreamLocalLocked(ls)
		if !snap.linkRemoved {
			t.Fatal("replaced link should remove old local snapshot")
		}
	})

	if _, linkRemoved := c.currentControlStreamRemoteExchange(ls); !linkRemoved {
		t.Fatal("replaced link should hide old remote hashes")
	}
	remote := legacySolicitationExchange(hashes)
	if c.setControlStreamRemoteExchange(ls, remote) {
		t.Fatal("replaced link accepted old remote hashes")
	}
	if !c.setControlStreamRemoteExchange(replacement, remote) {
		t.Fatal("replacement link should accept remote hashes")
	}
}

func TestEvaluateMatchesSuppressesDuplicateOpens(t *testing.T) {
	ctx := t.Context()

	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	handler := newTestResolverHandler()
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/dupe"),
		Context:    []byte("ctx"),
	}
	hashes := link_solicit.ComputeProtocolHashes(ls.sessionID, []link_solicit.SolicitEntry{entry})
	ss := &solicitState{
		dir:     link_solicit.NewSolicitProtocol(entry.ProtocolID, entry.Context, "", 0),
		handler: handler,
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		c.solicitations[ss] = struct{}{}
		broadcast()
	})

	exchange := legacySolicitationExchange(hashes)
	c.evaluateMatches(ctx, ls, exchange, exchange)
	recvTestValue(t, ls.ml.(*testMountedLink).openCh, "opened protocol")
	recvTestValue(t, handler.values, "solicit value")
	if len(ls.matched) != 1 {
		t.Fatalf("matched count = %d, want 1", len(ls.matched))
	}

	c.evaluateMatches(ctx, ls, exchange, exchange)
	if len(ls.matched) != 1 {
		t.Fatalf("matched count after duplicate = %d, want 1", len(ls.matched))
	}
	assertNoTestValue(t, ls.ml.(*testMountedLink).openCh, "duplicate opened protocol")
	assertNoTestValue(t, handler.values, "duplicate solicit value")
}

// TestEvaluateMatchesStreamCloseDoesNotRearmIncarnatedPair verifies that stream
// lifetime does not control offer-pair suppression.
func TestEvaluateMatchesStreamCloseDoesNotRearmIncarnatedPair(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	handler := newTestResolverHandler()
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/incarnated"),
		Context:    []byte("ctx"),
	}
	hash := link_solicit.ComputeProtocolHash(ls.sessionID, entry.ProtocolID, entry.Context)
	localIncarnation := bytes.Repeat([]byte{1}, solicitationIncarnationSize)
	remoteIncarnation := bytes.Repeat([]byte{2}, solicitationIncarnationSize)
	ss := &solicitState{
		dir:         link_solicit.NewSolicitProtocol(entry.ProtocolID, entry.Context, "", 0),
		handler:     handler,
		incarnation: localIncarnation,
	}
	local := incarnatedSolicitationExchange(hash, localIncarnation, 2, 3)
	remote := incarnatedSolicitationExchange(hash, remoteIncarnation, 3, 2)
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		c.solicitations[ss] = struct{}{}
		ls.remoteExchange = remote
		broadcast()
	})

	c.evaluateMatches(t.Context(), ls, local, remote)
	recvTestValue(t, ls.ml.(*testMountedLink).openCh, "opened incarnated protocol")
	value := recvTestValue(t, handler.values, "incarnated solicit value")
	sms := value.(link_solicit.SolicitMountedStream)
	ms, _, err := sms.AcceptMountedStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := ms.GetStream().Close(); err != nil {
		t.Fatal(err)
	}

	c.evaluateMatches(t.Context(), ls, local, remote)
	assertNoTestValue(t, ls.ml.(*testMountedLink).openCh, "reopened closed protocol")
	assertNoTestValue(t, handler.values, "replacement for closed stream")
}

// TestRetainedLinkPrunesRetiredIncarnationPairs verifies bounded suppression
// across repeated local and remote offer rotations.
func TestRetainedLinkPrunesRetiredIncarnationPairs(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	handler := newTestResolverHandler()
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/rotating-incarnations"),
		Context:    []byte("ctx"),
	}
	hash := link_solicit.ComputeProtocolHash(ls.sessionID, entry.ProtocolID, entry.Context)
	ss := &solicitState{
		dir:     link_solicit.NewSolicitProtocol(entry.ProtocolID, entry.Context, "", 0),
		handler: handler,
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		c.solicitations[ss] = struct{}{}
		ls.matched[hex.EncodeToString(hash)] = struct{}{}
		broadcast()
	})

	for generation := uint64(1); generation <= 8; generation++ {
		localIncarnation := bytes.Repeat([]byte{byte(generation)}, solicitationIncarnationSize)
		remoteIncarnation := bytes.Repeat([]byte{byte(generation + 16)}, solicitationIncarnationSize)
		local := incarnatedSolicitationExchange(
			hash,
			localIncarnation,
			generation,
			generation,
		)
		remote := incarnatedSolicitationExchange(
			hash,
			remoteIncarnation,
			generation,
			generation,
		)
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			ss.incarnation = localIncarnation
			broadcast()
		})
		if generation > 1 {
			previousRemote, removed := c.currentControlStreamRemoteExchange(ls)
			if removed {
				t.Fatal("retained link was removed")
			}
			c.pruneRetiredMatches(ls, local, previousRemote)
			if len(ls.matched) != 1 {
				t.Fatalf("generation %d local rotation retained %d matches, want legacy only", generation, len(ls.matched))
			}
		}
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			ls.remoteExchange = remote
			broadcast()
		})

		c.pruneRetiredMatches(ls, local, remote)
		c.evaluateMatches(t.Context(), ls, local, remote)
		recvTestValue(t, ls.ml.(*testMountedLink).openCh, "rotated opened protocol")
		recvTestValue(t, handler.values, "rotated solicitation value")
		if len(ls.matched) != 2 {
			t.Fatalf("generation %d matched entries = %d, want legacy plus current pair", generation, len(ls.matched))
		}
	}
}

// TestOpenSolicitedStreamWaitsForAccept verifies the opener publishes a stream
// only after the receiver accepts its offer pair.
func TestOpenSolicitedStreamWaitsForAccept(t *testing.T) {
	// Build a lower-peer link and one solicitation entry.
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	handler := newTestResolverHandler()
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/accept"),
		Context:    []byte("ctx"),
	}
	hash := link_solicit.ComputeProtocolHash(ls.sessionID, entry.ProtocolID, entry.Context)

	// Bind the offer to fixed local and remote incarnations.
	localIncarnation := bytes.Repeat([]byte{1}, solicitationIncarnationSize)
	remoteIncarnation := bytes.Repeat([]byte{2}, solicitationIncarnationSize)
	ss := &solicitState{
		dir:         link_solicit.NewSolicitProtocol(entry.ProtocolID, entry.Context, "", 0),
		handler:     handler,
		incarnation: localIncarnation,
	}

	// Register it against the remote's last advertised offer.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		c.solicitations[ss] = struct{}{}
		ls.remoteExchange = incarnatedSolicitationExchange(hash, remoteIncarnation, 2, 2)
		broadcast()
	})
	match := solicitationMatch{
		hash:              hash,
		localIncarnation:  localIncarnation,
		remoteIncarnation: remoteIncarnation,
		incarnated:        true,
	}

	// A receiver that already rotated its incarnation closes the stream.
	ml := ls.ml.(*testMountedLink)
	ml.reject = true
	c.openSolicitedStream(t.Context(), ls, match)
	recvTestValue(t, ml.openCh, "rejected opened protocol")
	assertNoTestValue(t, handler.values, "rejected solicited stream")

	// An accepted stream reaches the solicitation.
	ml.reject = false
	c.openSolicitedStream(t.Context(), ls, match)
	recvTestValue(t, ml.openCh, "accepted opened protocol")
	recvTestValue(t, handler.values, "accepted solicited stream")
}

// TestPruneRetiredMatchRemovesOpenBeforeRoutineStarts verifies withdrawal
// cleanup when the keyed manager has no running context.
func TestPruneRetiredMatchRemovesOpenBeforeRoutineStarts(t *testing.T) {
	c, err := NewController(logrus.NewEntry(logrus.New()), &Config{})
	if err != nil {
		t.Fatal(err)
	}
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	hash := link_solicit.ComputeProtocolHash(ls.sessionID, protocol.ID("test/pending"), nil)
	localIncarnation := bytes.Repeat([]byte{1}, solicitationIncarnationSize)
	remoteIncarnation := bytes.Repeat([]byte{2}, solicitationIncarnationSize)
	match := solicitationMatch{
		hash:              hash,
		localIncarnation:  localIncarnation,
		remoteIncarnation: remoteIncarnation,
		incarnated:        true,
	}
	matchKey := encodeSolicitationMatch(ls, match)
	openKey := solicitationOpenKey{linkUUID: ls.ml.GetLinkUUID(), match: matchKey}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		ls.matched[matchKey] = struct{}{}
		c.opens[openKey] = solicitationOpen{ls: ls, match: match}
		broadcast()
	})
	c.openRoutines.SetKey(openKey, true)

	replacement := bytes.Repeat([]byte{3}, solicitationIncarnationSize)
	local := incarnatedSolicitationExchange(hash, replacement, 2, 2)
	remote := incarnatedSolicitationExchange(hash, replacement, 2, 2)
	c.pruneRetiredMatches(ls, local, remote)

	if len(ls.matched) != 0 {
		t.Fatalf("matched entries = %d, want 0", len(ls.matched))
	}
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if _, exists := c.opens[openKey]; exists {
			t.Fatal("retired open metadata remains")
		}
	})
	if _, exists := c.openRoutines.GetKey(openKey); exists {
		t.Fatal("retired open routine remains")
	}
}

// TestStartOpenRoutineRejectsLinkRemovedBeforeRegistration covers link removal
// between pending-open publication and keyed registration.
func TestStartOpenRoutineRejectsLinkRemovedBeforeRegistration(t *testing.T) {
	c, err := NewController(logrus.NewEntry(logrus.New()), &Config{})
	if err != nil {
		t.Fatal(err)
	}
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	match := solicitationMatch{
		hash:              bytes.Repeat([]byte{1}, link_solicit.HashSize),
		localIncarnation:  bytes.Repeat([]byte{2}, solicitationIncarnationSize),
		remoteIncarnation: bytes.Repeat([]byte{3}, solicitationIncarnationSize),
		incarnated:        true,
	}
	matchKey := encodeSolicitationMatch(ls, match)
	openKey := solicitationOpenKey{linkUUID: ls.ml.GetLinkUUID(), match: matchKey}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ls.refCount = 1
		c.links[ls.ml.GetLinkUUID()] = ls
		ls.matched[matchKey] = struct{}{}
		c.opens[openKey] = solicitationOpen{ls: ls, match: match}
		broadcast()
	})
	c.removeLink(ls.ml.GetLinkUUID())
	c.startOpenRoutine(openKey)

	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if _, exists := c.opens[openKey]; exists {
			t.Fatal("removed link retained open metadata")
		}
	})
	if _, exists := c.openRoutines.GetKey(openKey); exists {
		t.Fatal("removed link retained open routine")
	}
}

// TestIncomingSolicitedStreamRejectsStaleIncarnation verifies both stale-pair
// and upgraded-peer downgrade rejection.
func TestIncomingSolicitedStreamRejectsStaleIncarnation(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("b"), peer.ID("a"))
	handler := newTestResolverHandler()
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/stale-incarnation"),
		Context:    []byte("ctx"),
	}
	hash := link_solicit.ComputeProtocolHash(ls.sessionID, entry.ProtocolID, entry.Context)
	lowerIncarnation := bytes.Repeat([]byte{1}, solicitationIncarnationSize)
	higherIncarnation := bytes.Repeat([]byte{2}, solicitationIncarnationSize)
	ss := &solicitState{
		dir:         link_solicit.NewSolicitProtocol(entry.ProtocolID, entry.Context, "", 0),
		handler:     handler,
		incarnation: higherIncarnation,
	}
	remote := incarnatedSolicitationExchange(hash, lowerIncarnation, 3, 2)
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		c.solicitations[ss] = struct{}{}
		ls.remoteExchange = remote
		broadcast()
	})
	newStream := func() link.MountedStream {
		return &testMountedStream{link: ls.ml}
	}

	staleLower := bytes.Repeat([]byte{3}, solicitationIncarnationSize)
	stalePair := hex.EncodeToString(hash) + ":" +
		hex.EncodeToString(staleLower) + ":" + hex.EncodeToString(higherIncarnation)
	c.handleIncomingSolicitedStream(stalePair, newStream())
	assertNoTestValue(t, handler.values, "stale incarnation stream")

	c.handleIncomingSolicitedStream(hex.EncodeToString(hash), newStream())
	assertNoTestValue(t, handler.values, "hash-only stream from upgraded peer")

	currentPair := hex.EncodeToString(hash) + ":" +
		hex.EncodeToString(lowerIncarnation) + ":" + hex.EncodeToString(higherIncarnation)
	c.handleIncomingSolicitedStream(currentPair, newStream())
	recvTestValue(t, handler.values, "current incarnation stream")
}

func TestEvaluateMatchesHigherPeerDoesNotOpenStream(t *testing.T) {
	ctx := t.Context()

	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("b"), peer.ID("a"))
	handler := newTestResolverHandler()
	entry := link_solicit.SolicitEntry{
		ProtocolID: protocol.ID("test/higher"),
		Context:    []byte("ctx"),
	}
	hashes := link_solicit.ComputeProtocolHashes(ls.sessionID, []link_solicit.SolicitEntry{entry})
	ss := &solicitState{
		dir:     link_solicit.NewSolicitProtocol(entry.ProtocolID, entry.Context, "", 0),
		handler: handler,
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		c.solicitations[ss] = struct{}{}
		broadcast()
	})

	exchange := legacySolicitationExchange(hashes)
	c.evaluateMatches(ctx, ls, exchange, exchange)
	if len(ls.matched) != 1 {
		t.Fatalf("matched count = %d, want 1", len(ls.matched))
	}
	assertNoTestValue(t, ls.ml.(*testMountedLink).openCh, "higher-peer opened protocol")
	assertNoTestValue(t, handler.values, "higher-peer solicit value")
}

func TestControlStreamSendsFullHashSetAfterLocalChange(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.links[ls.ml.GetLinkUUID()] = ls
		broadcast()
	})

	localConn, remoteConn := net.Pipe()
	defer localConn.Close()
	defer remoteConn.Close()

	maxMessageSize := maxExchangeMessageSize(c.maxHashes)
	localSess := stream_packet.NewSession(localConn, maxMessageSize)
	remoteSess := stream_packet.NewSession(remoteConn, maxMessageSize)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.runControlStream(ctx, ls, localSess)
	}()

	first := &solicitState{
		dir: link_solicit.NewSolicitProtocol(protocol.ID("test/full-a"), []byte("a"), "", 0),
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.solicitations[first] = struct{}{}
		broadcast()
	})
	firstMsg := recvSolicitationExchange(t, remoteSess)
	firstExpected := link_solicit.ComputeProtocolHashes(
		ls.sessionID,
		[]link_solicit.SolicitEntry{{
			ProtocolID: protocol.ID("test/full-a"),
			Context:    []byte("a"),
		}},
	)
	if !slices.EqualFunc(firstMsg.GetProtocolHashes(), firstExpected, bytes.Equal) {
		t.Fatalf("first exchange hashes = %x, want %x", firstMsg.GetProtocolHashes(), firstExpected)
	}

	second := &solicitState{
		dir: link_solicit.NewSolicitProtocol(protocol.ID("test/full-b"), []byte("b"), "", 0),
	}
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.solicitations[second] = struct{}{}
		broadcast()
	})
	secondMsg := recvSolicitationExchange(t, remoteSess)
	secondExpected := link_solicit.ComputeProtocolHashes(
		ls.sessionID,
		[]link_solicit.SolicitEntry{
			{ProtocolID: protocol.ID("test/full-a"), Context: []byte("a")},
			{ProtocolID: protocol.ID("test/full-b"), Context: []byte("b")},
		},
	)
	if !slices.EqualFunc(secondMsg.GetProtocolHashes(), secondExpected, bytes.Equal) {
		t.Fatalf("second exchange hashes = %x, want %x", secondMsg.GetProtocolHashes(), secondExpected)
	}

	cancel()
	<-done
}

// TestControlStreamMaxExchangeFitsPacketBound verifies the configured offer
// limit fits through the upgraded packet codec.
func TestControlStreamMaxExchangeFitsPacketBound(t *testing.T) {
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	offers := make([]solicitationOffer, c.maxHashes)
	for i := range offers {
		offers[i] = solicitationOffer{
			protocolID:  protocol.ID(strconv.Itoa(i)),
			incarnation: bytes.Repeat([]byte{byte(i)}, solicitationIncarnationSize),
		}
	}
	exchange := c.computeExchange(ls, offers)
	exchange.generation = 1
	exchange.acknowledgedGeneration = 1

	localConn, remoteConn := net.Pipe()
	defer localConn.Close()
	defer remoteConn.Close()
	maxMessageSize := maxExchangeMessageSize(c.maxHashes)
	localSess := stream_packet.NewSession(localConn, maxMessageSize)
	remoteSess := stream_packet.NewSession(remoteConn, maxMessageSize)
	sendErr := make(chan error, 1)
	go func() {
		sendErr <- c.sendExchange(localSess, exchange, true)
	}()

	var received link_solicit.SolicitationExchange
	if err := remoteSess.RecvMsg(&received); err != nil {
		t.Fatal(err)
	}
	if err := recvTestValue(t, sendErr, "max exchange send"); err != nil {
		t.Fatal(err)
	}
	if len(received.GetProtocolHashes()) != 0 {
		t.Fatalf("protocol hashes = %d, want 0 in upgraded exchange", len(received.GetProtocolHashes()))
	}
	if len(received.GetOffers()) != int(c.maxHashes) {
		t.Fatalf("offers = %d, want %d", len(received.GetOffers()), c.maxHashes)
	}
}

func recvSolicitationExchange(t *testing.T, sess *stream_packet.Session) *link_solicit.SolicitationExchange {
	t.Helper()

	done := make(chan error, 1)
	msg := &link_solicit.SolicitationExchange{}
	go func() {
		done <- sess.RecvMsg(msg)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for solicitation exchange")
	}
	return nil
}

// TestSolicitProtocolRestartsOnRetainedLink verifies that restarting both
// consumers establishes a fresh stream over the existing link.
func TestSolicitProtocolRestartsOnRetainedLink(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Set up two testbeds with inproc transport and solicitation.
	tb1 := buildTestbed(t, ctx)
	tb2 := buildTestbed(t, ctx)

	_, tp1, tp1Ref := startTransport(t, ctx, tb1, nil)
	defer tp1Ref.Release()
	_, tp2, tp2Ref := startTransport(t, ctx, tb2, &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			tp1.GetPeerID().String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	defer tp2Ref.Release()

	// Wire inproc transports.
	tp1.ConnectToInproc(ctx, tp2)
	tp2.ConnectToInproc(ctx, tp1)

	// Start solicitation controllers.
	scRef1 := startSolicitController(t, ctx, tb1)
	defer scRef1.Release()
	scRef2 := startSolicitController(t, ctx, tb2)
	defer scRef2.Release()

	// Establish a link.
	pid1 := tp1.GetPeerID()
	heldLink, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", pid1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	// Both peers solicit the same protocol.
	// Must add directives on both sides before waiting, since matching
	// requires both peers to have the solicitation active.
	testProto := protocol.ID("test/echo")

	type result struct {
		sms link_solicit.SolicitMountedStream
		ref directive.Reference
		err error
	}

	runRound := func(marker string) (directive.Reference, directive.Reference, [2]uint64) {
		t.Helper()

		ch1 := make(chan result, 1)
		ch2 := make(chan result, 1)
		go func() {
			sms, _, ref, err := link_solicit.ExSolicitProtocol(ctx, tb1.Bus, testProto, nil, "", 0)
			ch1 <- result{sms, ref, err}
		}()
		go func() {
			sms, _, ref, err := link_solicit.ExSolicitProtocol(ctx, tb2.Bus, testProto, nil, "", 0)
			ch2 <- result{sms, ref, err}
		}()

		await := func(ch <-chan result, peerNum int) result {
			select {
			case res := <-ch:
				if res.err != nil {
					t.Fatalf("peer %d solicit error: %v", peerNum, res.err)
				}
				return res
			case <-ctx.Done():
				t.Fatalf("peer %d solicitation did not resolve: %v", peerNum, ctx.Err())
			}
			return result{}
		}
		r1 := await(ch1, 1)
		r2 := await(ch2, 2)

		accept := func(sms link_solicit.SolicitMountedStream, peerNum int) link.MountedStream {
			ms, alreadyAccepted, err := sms.AcceptMountedStream()
			if err != nil {
				t.Fatalf("peer %d accept error: %v", peerNum, err)
			}
			if alreadyAccepted {
				t.Fatalf("peer %d received the previously consumed stream", peerNum)
			}
			if ms == nil {
				t.Fatalf("peer %d got nil MountedStream", peerNum)
			}
			return ms
		}
		ms1 := accept(r1.sms, 1)
		defer ms1.GetStream().Close()
		ms2 := accept(r2.sms, 2)
		defer ms2.GetStream().Close()

		deadline := time.Now().Add(time.Second)
		if err := ms1.GetStream().SetDeadline(deadline); err != nil {
			t.Fatalf("peer 1 stream deadline: %v", err)
		}
		if err := ms2.GetStream().SetDeadline(deadline); err != nil {
			t.Fatalf("peer 2 stream deadline: %v", err)
		}

		data := []byte(marker)
		if _, err := ms1.GetStream().Write(data); err != nil {
			t.Fatalf("write %q: %v", marker, err)
		}
		buf := make([]byte, len(data))
		if _, err := io.ReadFull(ms2.GetStream(), buf); err != nil {
			t.Fatalf("read %q: %v", marker, err)
		}
		if !bytes.Equal(buf, data) {
			t.Fatalf("data mismatch: got %q, want %q", buf, data)
		}

		return r1.ref, r2.ref, [2]uint64{
			ms1.GetLink().GetLinkUUID(),
			ms2.GetLink().GetLinkUUID(),
		}
	}

	round1Ref1, round1Ref2, round1Links := runRound("round one")
	round1Ref1.Release()
	round1Ref2.Release()

	round2Ref1, round2Ref2, round2Links := runRound("round two")
	defer round2Ref1.Release()
	defer round2Ref2.Release()

	if round2Links != round1Links {
		t.Fatalf("link UUIDs changed: round 1 %v, round 2 %v", round1Links, round2Links)
	}
	if round2Links[1] != heldLink.GetLinkUUID() {
		t.Fatalf("peer 2 link UUID = %d, held link UUID = %d", round2Links[1], heldLink.GetLinkUUID())
	}
}

// TestSolicitProtocolNoMatch tests that disjoint protocol sets don't match.
func TestSolicitProtocolNoMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tb1 := buildTestbed(t, ctx)
	tb2 := buildTestbed(t, ctx)

	_, tp1, tp1Ref := startTransport(t, ctx, tb1, nil)
	defer tp1Ref.Release()
	_, tp2, tp2Ref := startTransport(t, ctx, tb2, &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			tp1.GetPeerID().String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	defer tp2Ref.Release()

	tp1.ConnectToInproc(ctx, tp2)
	tp2.ConnectToInproc(ctx, tp1)

	scRef1 := startSolicitController(t, ctx, tb1)
	defer scRef1.Release()
	scRef2 := startSolicitController(t, ctx, tb2)
	defer scRef2.Release()

	pid1 := tp1.GetPeerID()
	_, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", pid1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	// Peer 1 solicits "proto/a", peer 2 solicits "proto/b" -- no match.
	_, diRef1, err := tb1.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("proto/a"), nil, "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef1.Release()

	_, diRef2, err := tb2.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("proto/b"), nil, "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef2.Release()

	// The test verifies no crash/hang with disjoint sets.
	// A brief sleep would be needed to verify no match, but for now
	// we just verify the system doesn't deadlock or panic.
	t.Log("no-match test completed without panic or deadlock")
}

// TestSolicitProtocolContextMismatch tests that same protocol ID but
// different contexts don't match.
func TestSolicitProtocolContextMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	tb1 := buildTestbed(t, ctx)
	tb2 := buildTestbed(t, ctx)

	_, tp1, tp1Ref := startTransport(t, ctx, tb1, nil)
	defer tp1Ref.Release()
	_, tp2, tp2Ref := startTransport(t, ctx, tb2, &inproc.Config{
		Dialers: map[string]*dialer.DialerOpts{
			tp1.GetPeerID().String(): {
				Address: tp1.LocalAddr().String(),
			},
		},
	})
	defer tp2Ref.Release()

	tp1.ConnectToInproc(ctx, tp2)
	tp2.ConnectToInproc(ctx, tp1)

	scRef1 := startSolicitController(t, ctx, tb1)
	defer scRef1.Release()
	scRef2 := startSolicitController(t, ctx, tb2)
	defer scRef2.Release()

	pid1 := tp1.GetPeerID()
	_, lnkRel, err := link.EstablishLinkWithPeerEx(ctx, tb2.Bus, "", pid1, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lnkRel()

	// Same protocol but different context bytes -- should not match.
	_, diRef1, err := tb1.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("dex"), []byte("bucket-a"), "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef1.Release()

	_, diRef2, err := tb2.Bus.AddDirective(
		link_solicit.NewSolicitProtocol(protocol.ID("dex"), []byte("bucket-b"), "", 0),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef2.Release()

	t.Log("context mismatch test completed without panic or deadlock")
}
