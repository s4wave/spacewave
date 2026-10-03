package link_solicit_controller

import (
	"bytes"
	"context"
	"net"
	"testing"

	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestControlStreamUpgradeRequiresOfferAcknowledgement keeps a hash-only probe
// acknowledgement from opening a stream before the peer has its incarnation.
func TestControlStreamUpgradeRequiresOfferAcknowledgement(t *testing.T) {
	// Start one control loop with a local offer and a manually driven peer.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	c := newTestSolicitController(t)
	ls := newTestLinkState(peer.ID("a"), peer.ID("b"))
	ss := &solicitState{
		ctx:         t.Context(),
		dir:         link_solicit.NewSolicitProtocol(protocol.ID("test/upgrade"), nil, "", 0),
		incarnation: bytes.Repeat([]byte{1}, solicitationIncarnationSize),
		handler:     newTestResolverHandler(),
	}
	c.links[ls.ml.GetLinkUUID()] = ls
	c.solicitations[ss] = struct{}{}

	// Run the control loop over a pipe to the manually driven peer.
	localConn, remoteConn := net.Pipe()
	defer localConn.Close()
	defer remoteConn.Close()
	maxMessageSize := maxExchangeMessageSize(c.maxHashes)
	localSess := stream_packet.NewSession(localConn, maxMessageSize)
	remoteSess := stream_packet.NewSession(remoteConn, maxMessageSize)

	// Stop the control loop before the test returns.
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.runControlStream(ctx, ls, localSess)
	}()
	defer func() {
		cancel()
		<-done
	}()

	// Acknowledge only the probe, while advertising the peer's full offer.
	probe := recvSolicitationExchange(t, remoteSess)
	if len(probe.GetProtocolHashes()) != 1 || len(probe.GetOffers()) != 0 {
		t.Fatal("initial capability probe must carry only the legacy hash set")
	}
	remote := &link_solicit.SolicitationExchange{
		SupportsOfferIncarnations: true,
		Generation:                1,
		AcknowledgedGeneration:    probe.GetGeneration(),
		Offers: []*link_solicit.SolicitationOffer{{
			ProtocolHash: probe.GetProtocolHashes()[0],
			Incarnation:  bytes.Repeat([]byte{2}, solicitationIncarnationSize),
		}},
	}
	if err := remoteSess.SendMsg(remote); err != nil {
		t.Fatal(err)
	}

	// The first complete offer must require a distinct acknowledgement.
	offer := recvSolicitationExchange(t, remoteSess)
	if offer.GetGeneration() != probe.GetGeneration()+1 {
		t.Fatalf("offer generation = %d, want %d", offer.GetGeneration(), probe.GetGeneration()+1)
	}
	if len(offer.GetOffers()) != 1 || offer.GetAcknowledgedGeneration() != remote.GetGeneration() {
		t.Fatal("upgraded exchange must carry the local offer and acknowledge the peer")
	}
	if matches := findSolicitationMatches(c.decodeExchange(offer), c.decodeExchange(remote)); len(matches) != 0 {
		t.Fatal("probe acknowledgement matched an unacknowledged incarnation")
	}
	remote.AcknowledgedGeneration = offer.GetGeneration()
	if matches := findSolicitationMatches(c.decodeExchange(offer), c.decodeExchange(remote)); len(matches) != 1 {
		t.Fatal("complete offer acknowledgement did not match the protocol")
	}
}
