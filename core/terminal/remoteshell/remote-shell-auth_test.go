//go:build !js

package remoteshell

import (
	"context"
	"crypto/rand"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestRemoteShellRefusesPeerOutsideAccount checks that only a peer the local
// Session transport authorizes reaches the device policy.
func TestRemoteShellRefusesPeerOutsideAccount(t *testing.T) {
	// Start a testbed bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Run a Session transport that admits one account peer.
	member, outsider := newTestPeerID(t), newTestPeerID(t)
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	st, err := transport.NewSessionTransport(
		logrus.NewEntry(logrus.New()),
		tb.Bus,
		privKey,
		"",
		"",
		transport.WithPeerAuthorizer(func(_ context.Context, remote peer.ID) error {
			if remote != member {
				return errors.New("not an account session")
			}
			return nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = st.Execute(ctx) }()
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Serve each peer's stream through the handler the daemon registers.
	handler := &deviceRemoteShellHandler{
		le:        logrus.NewEntry(logrus.New()),
		b:         tb.Bus,
		authorize: authorizeAccountSessionPeer(tb.Bus),
		policy: func(*s4wave_terminal.TerminalFrame) error {
			return errors.New("policy reached")
		},
		starter: func(context.Context, *s4wave_terminal.TerminalFrame) (remoteShellProcess, error) {
			return nil, errors.New("unexpected start")
		},
	}
	for _, tc := range []struct {
		name   string
		remote peer.ID
		want   string
	}{
		{name: "outsider", remote: outsider, want: "remote shell refused"},
		{name: "member", remote: member, want: "policy reached"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := openTestRemoteShell(ctx, t, handler, st.GetPeerID(), tc.remote)
			if !strings.HasPrefix(got, tc.want) {
				t.Fatalf("error = %q, want prefix %q", got, tc.want)
			}
		})
	}
}

// openTestRemoteShell sends OPEN from remote to local and returns the error
// frame the handler answers with.
func openTestRemoteShell(
	ctx context.Context,
	t *testing.T,
	handler *deviceRemoteShellHandler,
	local, remote peer.ID,
) string {
	// Hand the server end of a pipe to the handler.
	t.Helper()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	ms := &testMountedStream{
		conn:   serverConn,
		remote: remote,
		link:   &testMountedLink{local: local, remote: remote},
	}
	if err := handler.HandleMountedStream(ctx, ms); err != nil {
		t.Fatal(err)
	}

	// A refused peer is answered before OPEN, so send it without waiting.
	client := stream_packet.NewSession(clientConn, deviceRemoteShellFrameMaxBytes)
	go func() {
		_ = client.SendMsg(&s4wave_terminal.TerminalFrame{
			Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OPEN,
		})
	}()
	got := &s4wave_terminal.TerminalFrame{}
	if err := client.RecvMsg(got); err != nil {
		t.Fatal(err)
	}
	if got.GetKind() != s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR {
		t.Fatalf("response kind = %s", got.GetKind().String())
	}
	return got.GetError()
}

// newTestPeerID returns a fresh peer identity.
func newTestPeerID(t *testing.T) peer.ID {
	// Derive the ID from a fresh key.
	t.Helper()
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// testMountedStream is a mounted stream over an in-memory connection.
type testMountedStream struct {
	link.MountedStream

	// conn carries the stream.
	conn net.Conn
	// remote is the remote peer.
	remote peer.ID
	// link is the link carrying the stream.
	link link.MountedLink
}

// GetStream returns the in-memory connection.
func (s *testMountedStream) GetStream() stream.Stream { return s.conn }

// GetPeerID returns the remote peer.
func (s *testMountedStream) GetPeerID() peer.ID { return s.remote }

// GetLink returns the link carrying the stream.
func (s *testMountedStream) GetLink() link.MountedLink { return s.link }

// testMountedLink is a link between two fixed peers.
type testMountedLink struct {
	link.MountedLink

	// local is the local peer.
	local peer.ID
	// remote is the remote peer.
	remote peer.ID
}

// GetLocalPeer returns the local peer.
func (l *testMountedLink) GetLocalPeer() peer.ID { return l.local }

// GetRemotePeer returns the remote peer.
func (l *testMountedLink) GetRemotePeer() peer.ID { return l.remote }
