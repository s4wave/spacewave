package transport_quic

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/s4wave/spacewave/net/crypto"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/sirupsen/logrus"
)

// TestMessageStream exchanges messages both ways on an unreliable stream and
// checks that each message stream receives only its own messages.
func TestMessageStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	links := newLinkPair(ctx, t, &Opts{DisablePathMtuDiscovery: true})

	var opened, accepted [2]stream.MessageStream
	for i := range opened {
		strm, err := links[0].OpenStream(stream.OpenOpts{Unreliable: true})
		if err != nil {
			t.Fatal(err)
		}
		msgs, ok := strm.(stream.MessageStream)
		if !ok {
			t.Fatalf("unreliable stream is %T", strm)
		}
		defer msgs.Close()
		opened[i] = msgs

		// The peer sees the stream once its control carries data.
		if _, err := msgs.Control().Write([]byte{byte(i)}); err != nil {
			t.Fatal(err)
		}
		control, _, err := links[1].AcceptStream()
		if err != nil {
			t.Fatal(err)
		}
		hello := make([]byte, 1)
		if _, err := io.ReadFull(control, hello); err != nil {
			t.Fatal(err)
		}
		accepted[hello[0]], err = links[1].AcceptMessageStream(control)
		if err != nil {
			t.Fatal(err)
		}
		defer accepted[hello[0]].Close()
	}

	for i := range opened {
		for _, pair := range [][2]stream.MessageStream{{opened[i], accepted[i]}, {accepted[i], opened[i]}} {
			want := []byte{'m', byte(i)}
			if _, err := pair[0].Write(want); err != nil {
				t.Fatal(err)
			}
			_ = pair[1].SetReadDeadline(time.Now().Add(2 * time.Second))
			got := make([]byte, 16)
			n, err := pair[1].Read(got)
			if err != nil {
				t.Fatal(err)
			}
			if string(got[:n]) != string(want) {
				t.Fatalf("stream %d received %q, want %q", i, got[:n], want)
			}
		}
	}

	// A message larger than one packet fails instead of fragmenting.
	var tooLarge *quic.DatagramTooLargeError
	if _, err := opened[0].Write(make([]byte, 4096)); !errors.As(err, &tooLarge) {
		t.Fatalf("oversized message: %v", err)
	}

	// Closing a message stream ends its reads.
	if err := accepted[1].Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := accepted[1].Read(make([]byte, 16)); err != io.EOF {
		t.Fatalf("read after close: %v", err)
	}
}

// TestMessageStreamRequiresDatagrams refuses an unreliable stream on a link
// without QUIC datagrams.
func TestMessageStreamRequiresDatagrams(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	links := newLinkPair(ctx, t, &Opts{DisablePathMtuDiscovery: true, DisableDatagrams: true})
	if _, err := links[0].OpenStream(stream.OpenOpts{Unreliable: true}); err != ErrDatagramsUnsupported {
		t.Fatalf("open unreliable stream: %v", err)
	}
}

// newLinkPair connects two links over loopback UDP. Index 0 dialed.
func newLinkPair(ctx context.Context, t *testing.T, opts *Opts) [2]*Link {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	var identities [2]*p2ptls.Identity
	var peers [2]peer.ID
	var endpoints [2]net.PacketConn
	for i := range identities {
		key, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		identities[i], err = p2ptls.NewIdentity(key)
		if err != nil {
			t.Fatal(err)
		}
		peers[i], err = peer.IDFromPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		endpoints[i], err = net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = endpoints[i].Close() })
	}

	listener, err := quic.Listen(endpoints[1], BuildIncomingTlsConf(identities[1], peers[0]), BuildQuicConfig(opts))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	dialer := &quic.Transport{Conn: endpoints[0]}
	t.Cleanup(func() { _ = dialer.Close() })
	outgoing, _, err := DialSessionViaTransport(ctx, le, opts, dialer, identities[0], endpoints[1].LocalAddr(), peers[1])
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := listener.Accept(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var links [2]*Link
	for side, conn := range []*quic.Conn{outgoing, incoming} {
		links[side], err = NewLink(ctx, le, opts, 0, peers[side], endpoints[side].LocalAddr(), conn, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = links[side].Close() })
	}
	return links
}
