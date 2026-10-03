package transport_quic

import (
	"context"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/s4wave/spacewave/net/crypto"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// recordingHandler records transport handler callbacks.
type recordingHandler struct {
	lost chan link.Link
}

// HandleLinkEstablished ignores established links.
func (h *recordingHandler) HandleLinkEstablished(link.Link) {}

// HandleLinkLost records the lost link.
func (h *recordingHandler) HandleLinkLost(lnk link.Link) { h.lost <- lnk }

// TestReplacedLinkReportsLoss verifies a session replaced at the same address
// reports its link loss to the transport handler.
func TestReplacedLinkReportsLoss(t *testing.T) {
	// Prepare peer identities and UDP endpoints for replacement sessions.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	var keys [2]crypto.PrivKey
	var identities [2]*p2ptls.Identity
	var peers [2]peer.ID
	var endpoints [2]net.PacketConn
	for i := range identities {
		// Generate the endpoint signing key and TLS identity.
		var err error
		keys[i], _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		identities[i], err = p2ptls.NewIdentity(keys[i])
		if err != nil {
			t.Fatal(err)
		}

		// Bind an authenticated peer to a local UDP endpoint.
		peers[i], err = peer.IDFromPrivateKey(keys[i])
		if err != nil {
			t.Fatal(err)
		}
		endpoints[i], err = net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer endpoints[i].Close()
	}

	// Start the QUIC listener and retain the outgoing transport.
	opts := &Opts{DisablePathMtuDiscovery: true}
	listener, err := quic.Listen(endpoints[1], BuildIncomingTlsConf(identities[1], peers[0]), BuildQuicConfig(opts))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	dialer := &quic.Transport{Conn: endpoints[0]}
	defer dialer.Close()

	// Record link loss from the transport accepting both sessions.
	handler := &recordingHandler{lost: make(chan link.Link, 4)}
	tpt, err := NewTransport(ctx, le, 0, endpoints[1].LocalAddr(), keys[1], handler, opts, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Establish two sessions through the same transport address.
	var lnks [2]*Link
	for i := range lnks {
		// Dial the authenticated listener from the shared UDP endpoint.
		outgoing, _, err := DialSessionViaTransport(ctx, le, opts, dialer, identities[0], endpoints[1].LocalAddr(), peers[1])
		if err != nil {
			t.Fatal(err)
		}
		defer outgoing.CloseWithError(0, "")

		// Accept the session and register its link with the transport.
		incoming, err := listener.Accept(ctx)
		if err != nil {
			t.Fatal(err)
		}
		lnks[i], err = tpt.HandleSession(ctx, incoming)
		if err != nil {
			t.Fatal(err)
		}
		defer lnks[i].Close()
	}

	// Require loss notification for the session replaced at this address.
	select {
	case lost := <-handler.lost:
		if lost != lnks[0] {
			t.Fatal("unexpected lost link")
		}
	case <-ctx.Done():
		t.Fatal("replaced link loss was not reported")
	}

	// Keep the replacement link registered after the prior link closes.
	if got, _ := tpt.LookupLinkWithAddr(lnks[1].RemoteAddr().String()); got != lnks[1] {
		t.Fatal("expected replacement link to remain registered")
	}
}
