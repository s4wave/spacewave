//go:build !goscript

package sobject_sync

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	bus_inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/callback"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// TestSOSyncExecuteRenewsOnlyFailedPeer verifies that a recoverable failure of
// one peer's stream renews an offer scoped to that peer alone, leaves another
// peer's running stream open, and that shutdown joins that stream.
func TestSOSyncExecuteRenewsOnlyFailedPeer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Pair the standing offer with two peers. Peer A's remote end is already
		// closed, so its stream fails at once; peer B's remote end never reads,
		// so its stream stays in authentication.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		le := gateLogger()
		b := bus_inmem.NewBus(directive_controller.NewController(ctx, le))
		failed, failedRemote := newTrackedMountedStream("peer-a")
		_ = failedRemote.Close()
		running, runningRemote := newTrackedMountedStream("peer-b")
		defer runningRemote.Close()

		// Resolve the standing offer with both streams and report each renewal.
		renewed := make(chan peer.ID, 4)
		info := controller.NewInfo(
			"test/so-sync-peer-retry",
			controller.MustParseVersion("0.0.1"),
			"SO sync peer retry test",
		)
		resolver := callback.NewCallbackController(
			info,
			nil,
			func(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
				dir, ok := di.GetDirective().(link_solicit.SolicitProtocol)
				if !ok {
					return nil, nil
				}
				if remote := dir.SolicitProtocolPeerID(); remote != "" {
					renewed <- remote
					return nil, nil
				}
				return []directive.Resolver{directive.NewValueResolver(
					[]link_solicit.SolicitMountedStream{
						link_solicit.NewSolicitMountedStream(failed),
						link_solicit.NewSolicitMountedStream(running),
					},
				)}, nil
			},
			nil,
		)
		releaseResolver, err := b.AddController(ctx, resolver, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseResolver()

		// Run the production Execute loop with a participant key, so each
		// stream reaches its transport.
		key := mustKeyPair(t)
		localID, err := peer.IDFromPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		links := newTestLinks("peer-a", "peer-b")
		syncer := NewSOSync(le, b, links, "test-object", localID, key, nil, nopPeerObserver{})
		errCh := make(chan error, 1)
		go func() {
			errCh <- syncer.Execute(ctx)
		}()

		// The failed peer renews its own offer after its backoff.
		if got := <-renewed; got != "peer-a" {
			t.Fatalf("expected a renewal for peer-a, got %q", got)
		}
		synctest.Wait()
		if len(renewed) != 0 {
			t.Fatalf("expected one renewal, got another for %q", <-renewed)
		}
		if running.closed.Load() {
			t.Fatal("peer-a's failure closed peer-b's running stream")
		}

		// Shutdown returns only after peer-b's stream has been closed and joined.
		cancel()
		if err := <-errCh; !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
		if !running.closed.Load() {
			t.Fatal("Execute returned before joining peer-b's stream")
		}
	})
}

// TestPeerStreamsEndRenewsWhileLinked verifies that the last stream with a
// linked peer renews its offer after a recoverable failure or an end that a
// held-state event can lift, not after the remote side's denial, and that the
// renewal ends with the peer's last link.
func TestPeerStreamsEndRenewsWhileLinked(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		linked bool
		renew  bool
	}{
		{name: "recoverable", err: errors.New("peer snapshot conflicts with accepted root"), linked: true, renew: true},
		{name: "recoverable without link", err: errors.New("peer snapshot conflicts with accepted root")},
		{name: "local refusal", err: &localRefusal{remoteID: "participant"}, linked: true, renew: true},
		{name: "remote denial", err: ErrAccessDenied, linked: true},
		{name: "participant revoked", err: sobject.ErrParticipantRevoked, linked: true, renew: true},
		{name: "history recovery", err: sobject.ErrConfigHistoryUnavailable, linked: true, renew: true},
		{name: "state too large", err: sobject.ErrStateTooLarge, linked: true, renew: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// End the only stream with a peer. The renewal's first delay and
				// the held-state watch both stay blocked until stop.
				ctx, cancel := context.WithCancel(t.Context())
				host, _ := newMemHost("test-object", &sobject.SOState{})
				links := newTestLinks()
				if test.linked {
					links.set("peer-a", true)
				}
				syncer := NewSOSync(gateLogger(), nil, links, "test-object", "", nil, host, nopPeerObserver{})
				streams := newPeerStreams(ctx, syncer)
				streams.arrive("peer-a")
				streams.end("peer-a", true, test.err)
				synctest.Wait()

				// Only a linked peer with a next step keeps a renewal.
				renewing := func() bool {
					var renewing bool
					streams.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
						renewing = streams.peers["peer-a"].renewal != nil
					})
					return renewing
				}
				if got := renewing(); got != test.renew {
					t.Fatalf("expected renewal %v, got %v", test.renew, got)
				}

				// The peer's last link going ends its renewal.
				links.set("peer-a", false)
				synctest.Wait()
				if renewing() {
					t.Fatal("renewal outlived the peer's last link")
				}

				// Stop joins the renewal and leaves new streams fenced.
				streams.stop(cancel)
				if streams.addWorker() {
					t.Fatal("stopped owner admitted another stream")
				}
			})
		})
	}
}

// TestSyncRetryBackoffIsCapped verifies the exact retry-delay ceiling.
func TestSyncRetryBackoffIsCapped(t *testing.T) {
	// Construct the policy and its expected capped sequence.
	backoff := newSyncRetryBackoff()
	want := []time.Duration{
		250 * time.Millisecond,
		500 * time.Millisecond,
		time.Second,
		2 * time.Second,
		2 * time.Second,
	}

	// Compare every delay through and beyond the ceiling.
	for i, expected := range want {
		if got := backoff.NextBackOff(); got != expected {
			t.Fatalf("delay %d: expected %s, got %s", i, expected, got)
		}
	}
}

// TestLogStreamResultReportsEachFailureChange verifies that a peer's blocked
// synchronization reaches the warn log once, however often the peer retries,
// and that recovery reaches the info log.
func TestLogStreamResultReportsEachFailureChange(t *testing.T) {
	// Capture the info and warn log of a syncer.
	log, hook := logtest.NewNullLogger()
	log.SetLevel(logrus.InfoLevel)
	le := logrus.NewEntry(log)
	syncer := NewSOSync(le, nil, nil, "test-object", "", nil, nil, nopPeerObserver{})

	// A terminal failure warns on its first stream and stays quiet after.
	blocked := errors.Wrap(sobject.ErrStateTooLarge, "12582912 bytes")
	for range 3 {
		syncer.logStreamResult(le, "peer-a", blocked)
	}
	if got := hook.AllEntries(); len(got) != 1 || got[0].Level != logrus.WarnLevel {
		t.Fatalf("repeated terminal failure logged %v; want one warning", got)
	}

	// Another peer is tracked on its own.
	hook.Reset()
	syncer.logStreamResult(le, "peer-b", blocked)
	if got := hook.AllEntries(); len(got) != 1 || got[0].Level != logrus.WarnLevel {
		t.Fatalf("second peer logged %v; want one warning", got)
	}

	// A clean stream reports recovery once, and a later failure warns again.
	hook.Reset()
	syncer.logStreamResult(le, "peer-a", nil)
	syncer.logStreamResult(le, "peer-a", nil)
	syncer.logStreamResult(le, "peer-a", blocked)
	got := hook.AllEntries()
	if len(got) != 2 || got[0].Level != logrus.InfoLevel || got[1].Level != logrus.WarnLevel {
		t.Fatalf("recovery and relapse logged %v; want info, warning", got)
	}

	// A recoverable failure that changes is reported at info.
	hook.Reset()
	syncer.logStreamResult(le, "peer-a", errors.New("EOF"))
	if got := hook.AllEntries(); len(got) != 1 || got[0].Level != logrus.InfoLevel {
		t.Fatalf("recoverable failure logged %v; want one info", got)
	}
}

// trackedMountedStream is a mounted stream over one end of a pipe that
// records whether its stream was closed.
type trackedMountedStream struct {
	// conn is the local end of the pipe.
	conn net.Conn
	// remote is the transport peer at the other end.
	remote peer.ID
	// closed is set once the stream is closed.
	closed atomic.Bool
}

// newTrackedMountedStream constructs a mounted stream with remote over a pipe
// and returns the pipe's remote end.
func newTrackedMountedStream(remote peer.ID) (*trackedMountedStream, net.Conn) {
	local, remoteConn := net.Pipe()
	return &trackedMountedStream{conn: local, remote: remote}, remoteConn
}

// GetStream returns the local end, recording its close.
func (s *trackedMountedStream) GetStream() stream.Stream {
	return trackedStream{Conn: s.conn, closed: &s.closed}
}

// GetProtocolID returns the sync protocol.
func (s *trackedMountedStream) GetProtocolID() protocol.ID {
	return SyncProtocolID
}

// GetOpenOpts returns the default open options.
func (s *trackedMountedStream) GetOpenOpts() stream.OpenOpts {
	return stream.OpenOpts{}
}

// GetPeerID returns the remote transport peer.
func (s *trackedMountedStream) GetPeerID() peer.ID {
	return s.remote
}

// GetLink returns a link from the local transport peer to remote.
func (s *trackedMountedStream) GetLink() link.MountedLink {
	return trackedMountedLink{remote: s.remote}
}

// trackedStream records the close of a pipe end.
type trackedStream struct {
	net.Conn
	// closed is set on Close.
	closed *atomic.Bool
}

// Close records the close and closes the pipe end.
func (s trackedStream) Close() error {
	s.closed.Store(true)
	return s.Conn.Close()
}

// trackedMountedLink is a link from transport peer "local" to remote.
type trackedMountedLink struct {
	// remote is the remote transport peer.
	remote peer.ID
}

// GetLinkUUID returns a fixed link ID.
func (trackedMountedLink) GetLinkUUID() uint64 { return 1 }

// GetTransportUUID returns a fixed transport ID.
func (trackedMountedLink) GetTransportUUID() uint64 { return 1 }

// GetRemoteTransportUUID returns a fixed transport ID.
func (trackedMountedLink) GetRemoteTransportUUID() uint64 { return 1 }

// GetLocalPeer returns the local transport peer, which sorts below the remote.
func (trackedMountedLink) GetLocalPeer() peer.ID { return "local" }

// GetRemotePeer returns the remote transport peer.
func (l trackedMountedLink) GetRemotePeer() peer.ID { return l.remote }

// OpenMountedStream is not used by the tests.
func (trackedMountedLink) OpenMountedStream(context.Context, protocol.ID, stream.OpenOpts) (link.MountedStream, error) {
	return nil, errors.New("not supported")
}

// testLinks is a PeerLinks whose linked peers a test sets.
type testLinks struct {
	// bcast guards linked and wakes link watches.
	bcast broadcast.Broadcast
	// linked holds the peers with a link.
	linked map[peer.ID]bool
}

// newTestLinks constructs a PeerLinks with a link to each of peers.
func newTestLinks(peers ...peer.ID) *testLinks {
	l := &testLinks{linked: make(map[peer.ID]bool)}
	for _, p := range peers {
		l.linked[p] = true
	}
	return l
}

// set links or unlinks p and wakes the watches.
func (l *testLinks) set(p peer.ID, linked bool) {
	l.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		l.linked[p] = linked
		bcast()
	})
}

// GetLinkedPeerIDsSnapshotWithWait returns the linked peers of peerIDs.
func (l *testLinks) GetLinkedPeerIDsSnapshotWithWait(peerIDs []peer.ID) (map[peer.ID]struct{}, []<-chan struct{}) {
	linked := make(map[peer.ID]struct{})
	var waitCh <-chan struct{}
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
		for _, p := range peerIDs {
			if l.linked[p] {
				linked[p] = struct{}{}
			}
		}
	})
	return linked, []<-chan struct{}{waitCh}
}

// _ is a type assertion.
var (
	_ link.MountedStream = (*trackedMountedStream)(nil)
	_ link.MountedLink   = trackedMountedLink{}
	_ PeerLinks          = (*testLinks)(nil)
)
