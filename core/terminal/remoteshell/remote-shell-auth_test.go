//go:build !js

package remoteshell

import (
	"context"
	"crypto/rand"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// TestRemoteShellServesOnlyWithNode checks that a Session answers remote shell
// streams only while its remote-shell controller runs, and then only for a peer
// the Session transport authorizes. Removing the controller stops new opens.
func TestRemoteShellServesOnlyWithNode(t *testing.T) {
	// The admitted peer starts a PTY shell, which Windows does not offer.
	if runtime.GOOS == "windows" {
		t.Skip("no PTY shell")
	}

	// Start a testbed bus.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Run a Session transport that admits one account peer and carries the
	// remote shell factory on its child bus.
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
		transport.WithChildFactories(func(b bus.Bus) controller.Factory { return NewFactory(b) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = st.Execute(ctx) }()
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	childBus := st.GetChildBus()

	// Without the node no handler answers the protocol.
	handlers, _, ref, err := bus.ExecCollectValues[link.MountedStreamHandler](
		ctx,
		childBus,
		link.NewHandleMountedStream(s4wave_terminal.RemoteShellProtocolID, st.GetPeerID(), member),
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	ref.Release()
	if len(handlers) != 0 {
		t.Fatalf("%d handlers resolved without the remote-shell node", len(handlers))
	}

	// The node's entry loads the controller on the Session bus.
	_, loadRef, err := childBus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer loadRef.Release()

	// Watch a handler the controller offers, to learn when it stops serving.
	removed := make(chan struct{})
	_, _, watchRef, err := bus.ExecCollectValues[link.MountedStreamHandler](
		ctx,
		childBus,
		link.NewHandleMountedStream(s4wave_terminal.RemoteShellProtocolID, st.GetPeerID(), member),
		true,
		func() { close(removed) },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer watchRef.Release()
	for _, tc := range []struct {
		name   string
		remote peer.ID
		want   s4wave_terminal.TerminalFrameKind
	}{
		{name: "outsider", remote: outsider, want: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR},
		{name: "member", remote: member, want: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Resolve the handler the Session bus offers this peer.
			handlers, _, ref, err := bus.ExecCollectValues[link.MountedStreamHandler](
				ctx,
				childBus,
				link.NewHandleMountedStream(s4wave_terminal.RemoteShellProtocolID, st.GetPeerID(), tc.remote),
				true,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer ref.Release()

			// Open a shell as the peer and check the answer.
			got := openTestRemoteShell(ctx, t, handlers[0], st.GetPeerID(), tc.remote)
			if got.GetKind() != tc.want {
				t.Fatalf("response = %s %q, want kind %s", got.GetKind(), got.GetError(), tc.want)
			}
			if tc.want == s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_ERROR &&
				!strings.HasPrefix(got.GetError(), "remote shell refused") {
				t.Fatalf("error = %q", got.GetError())
			}
		})
	}

	// Removing the node stops the controller, and a new open finds no handler.
	loadRef.Release()
	select {
	case <-removed:
	case <-ctx.Done():
		t.Fatal("handler still offered after the remote-shell node was removed")
	}
	handlers, _, ref, err = bus.ExecCollectValues[link.MountedStreamHandler](
		ctx,
		childBus,
		link.NewHandleMountedStream(s4wave_terminal.RemoteShellProtocolID, st.GetPeerID(), member),
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	ref.Release()
	if len(handlers) != 0 {
		t.Fatalf("%d handlers resolved after the remote-shell node was removed", len(handlers))
	}
}

// openTestRemoteShell sends OPEN from remote to local and returns the first
// frame the handler answers with.
func openTestRemoteShell(
	ctx context.Context,
	t *testing.T,
	handler link.MountedStreamHandler,
	local, remote peer.ID,
) *s4wave_terminal.TerminalFrame {
	// Hand the server end of a pipe to the handler.
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { _ = clientConn.Close() })
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
	return got
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

// GetProtocolID returns the remote shell protocol.
func (s *testMountedStream) GetProtocolID() protocol.ID { return s4wave_terminal.RemoteShellProtocolID }

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
