package provider_local

import (
	"context"
	"net"
	"testing"

	"github.com/quic-go/quic-go"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	"github.com/s4wave/spacewave/net/util/rwc"
)

// authenticatedPairingLinks creates the real authenticated QUIC links handed to
// StartDirect, with a bounded in-memory packet carrier in place of WebRTC.
func authenticatedPairingLinks(ctx context.Context, t *testing.T, source, receiving *Session) [2]link.Link {
	// Connect distinct authenticated Session endpoints without a registered route.
	t.Helper()
	left, right := net.Pipe()
	t.Cleanup(func() {
		// Close both ends even when the handshake never produced links.
		_ = left.Close()
		_ = right.Close()
	})
	peers := [2]peer.ID{source.GetPeerId(), receiving.GetPeerId()}
	addresses := [2]net.Addr{peer.NewNetAddr(peers[0]), peer.NewNetAddr(peers[1])}
	packets := [2]net.PacketConn{
		rwc.NewPacketConn(ctx, left, addresses[0], addresses[1], 65536, 10),
		rwc.NewPacketConn(ctx, right, addresses[1], addresses[0], 65536, 10),
	}

	// Bind the TLS identities to the same keys the pairing engines use.
	var identities [2]*p2ptls.Identity
	for i, session := range []*Session{source, receiving} {
		// Authenticate each endpoint with its existing Session credential.
		var err error
		identities[i], err = p2ptls.NewIdentity(session.GetPrivKey())
		if err != nil {
			t.Fatal(err)
		}
	}

	// Complete the actual mutually authenticated QUIC handshake concurrently.
	opts := &transport_quic.Opts{DisableDatagrams: true, DisableKeepAlive: true, DisablePathMtuDiscovery: true}
	accepted := make(chan struct {
		connection *quic.Conn
		err        error
	}, 1)
	go func() {
		// Deliver the accepted session or failure to the dialing side.
		connection, err := transport_quic.ListenSession(ctx, source.tkr.a.le, opts, packets[0], identities[0], peers[1])
		accepted <- struct {
			connection *quic.Conn
			err        error
		}{connection, err}
	}()
	outgoing, _, err := transport_quic.DialSession(ctx, receiving.tkr.a.le, opts, packets[1], identities[1], addresses[0], peers[0])
	if err != nil {
		t.Fatal(err)
	}
	incoming := <-accepted
	if incoming.err != nil {
		t.Fatal(incoming.err)
	}

	// Transfer each connection to the same raw link implementation as manual pairing.
	connections := [2]*quic.Conn{incoming.connection, outgoing}
	var links [2]link.Link
	for i, connection := range connections {
		lnk, err := transport_quic.NewLink(ctx, source.tkr.a.le, opts, 0, peers[i], addresses[i], connection, func() { _ = packets[i].Close() })
		if err != nil {
			t.Fatal(err)
		}
		links[i] = lnk
		t.Cleanup(func() { _ = lnk.Close() })
	}
	return links
}
