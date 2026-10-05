//go:build !tinygo

package transport_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// TestSessionTransportUDPAuthorizesPeers checks that a UDP link mounts only
// after the listening Session's authorizer admits the dialing peer.
func TestSessionTransportUDPAuthorizesPeers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		admit bool
	}{
		{name: "refused", admit: false},
		{name: "admitted", admit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build a listener that reports each decision and a dialer that
			// admits any peer.
			var dialerID peer.ID
			asked := make(chan peer.ID, 1)
			ctx, _, listener := newTestSessionTransport(t, "", transport.WithPeerAuthorizer(
				func(_ context.Context, remote peer.ID) error {
					select {
					case asked <- remote:
					default:
					}
					if !tc.admit || remote != dialerID {
						return errors.New("not an account session")
					}
					return nil
				},
			))
			_, _, dialer := newTestSessionTransport(t, "", transport.WithPeerAuthorizer(
				func(context.Context, peer.ID) error { return nil },
			))
			dialerID = dialer.GetPeerID()
			for _, st := range []*transport.SessionTransport{listener, dialer} {
				go func() { _ = st.Execute(ctx) }()
				if err := st.AwaitReady(ctx); err != nil {
					t.Fatal(err)
				}
			}

			// Listen on loopback and dial the listener from the other Session.
			addr, err := listener.StartUDP(ctx, "127.0.0.1:0", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := dialer.StartUDP(ctx, "127.0.0.1:0", map[peer.ID]string{
				listener.GetPeerID(): addr.String(),
			}); err != nil {
				t.Fatal(err)
			}
			_, elRef, err := dialer.GetChildBus().AddDirective(
				link.NewEstablishLinkWithPeer(dialerID, listener.GetPeerID()),
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer elRef.Release()

			// The listener decides on the dialer before any link mounts.
			select {
			case remote := <-asked:
				if remote != dialerID {
					t.Fatalf("asked about %s, want %s", remote, dialerID)
				}
			case <-ctx.Done():
				t.Fatal("listener never authorized the dialer")
			}
			if !tc.admit {
				linked, _ := listener.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{dialerID})
				if _, ok := linked[dialerID]; ok {
					t.Fatal("refused peer mounted a link")
				}
				return
			}

			// An admitted dialer links.
			for {
				linked, waitChs := listener.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{dialerID})
				if _, ok := linked[dialerID]; ok {
					return
				}
				select {
				case <-waitAny(waitChs):
				case <-ctx.Done():
					t.Fatal("admitted peer did not link")
				}
			}
		})
	}
}

// TestSessionTransportReauthorizeClosesRevokedPeer checks that a change from
// the authorization watch closes the link and admitted stream of a peer the
// authorizer no longer admits.
func TestSessionTransportReauthorizeClosesRevokedPeer(t *testing.T) {
	// Build a listener whose rule refuses once revoked is set, and whose
	// watch reports the change when revoke closes.
	var revoked atomic.Bool
	revoke := make(chan struct{})
	ctx, _, listener := newTestSessionTransport(t, "",
		transport.WithPeerAuthorizer(func(context.Context, peer.ID) error {
			if revoked.Load() {
				return errors.New("session revoked")
			}
			return nil
		}),
		transport.WithPeerAuthorizationWatch(func(ctx context.Context, changed func()) error {
			select {
			case <-revoke:
				changed()
			case <-ctx.Done():
			}
			return nil
		}),
	)
	_, _, dialer := newTestSessionTransport(t, "", transport.WithPeerAuthorizer(
		func(context.Context, peer.ID) error { return nil },
	))
	dialerID := dialer.GetPeerID()
	for _, st := range []*transport.SessionTransport{listener, dialer} {
		go func() { _ = st.Execute(ctx) }()
		if err := st.AwaitReady(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Hold each admitted stream open on the listener.
	const protocolID = protocol.ID("test/hold")
	admitted := make(chan stream.Stream, 1)
	release, err := listener.ServeAuthorized(
		protocolID,
		holdStreamHandler(admitted),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Listen and dial over loopback UDP.
	addr, err := listener.StartUDP(ctx, "127.0.0.1:0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dialer.StartUDP(ctx, "127.0.0.1:0", map[peer.ID]string{
		listener.GetPeerID(): addr.String(),
	}); err != nil {
		t.Fatal(err)
	}

	// Open a held stream; the first write delivers it to the listener.
	ms, msRel, err := link.OpenStreamWithPeerEx(
		ctx,
		dialer.GetChildBus(),
		protocolID,
		dialerID,
		listener.GetPeerID(),
		0,
		stream.OpenOpts{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer msRel()
	strm := ms.GetStream()
	defer strm.Close()
	if _, err := strm.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-admitted:
	case <-ctx.Done():
		t.Fatal("listener never admitted the stream")
	}

	// Revoke the dialer: its open stream ends and its link unmounts.
	revoked.Store(true)
	close(revoke)
	deadline, _ := ctx.Deadline()
	if err := strm.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	if _, err := strm.Read(make([]byte, 1)); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("stream read after revocation: %v, want a closed stream", err)
	}
	for {
		linked, waitChs := listener.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{dialerID})
		if _, ok := linked[dialerID]; !ok {
			return
		}
		select {
		case <-waitAny(waitChs):
		case <-ctx.Done():
			t.Fatal("revoked peer kept its link")
		}
	}
}

// holdStreamHandler sends each handled stream to its channel and leaves it open.
type holdStreamHandler chan<- stream.Stream

// HandleMountedStream sends the stream to the channel.
func (h holdStreamHandler) HandleMountedStream(_ context.Context, ms link.MountedStream) error {
	h <- ms.GetStream()
	return nil
}
