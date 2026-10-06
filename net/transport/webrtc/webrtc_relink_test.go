//go:build !js

package webrtc_test

import (
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/sirupsen/logrus"
)

// relinkBound is how long the peers may take to link again once the signaling
// path of the restarted peer is back.
const relinkBound = 15 * time.Second

// TestTransportRelinksAfterDroppedOffer verifies that the peers link again
// when the signaling relay drops the offer sent to a restarted peer. The
// restarted answerer asks for an offer once and never again, so the offerer
// must keep its replacement offer outstanding until the answer arrives.
func TestTransportRelinksAfterDroppedOffer(t *testing.T) {
	// Configure logging for transport negotiation and recovery.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Route the ICE data path over a vnet with an address for each transport
	// the test runs: both initial transports and the restarted one.
	iceNet := newICENetwork(t, 3)

	// Construct the two peers. They share no LAN, so only WebRTC links them.
	g := graph.NewGraph()
	var gp [2]*graph.Peer
	for i := range gp {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		gp[i] = p
	}
	sim := initSimulator(t, ctx, le, g)
	px := [2]*simulate.Peer{sim.GetPeerByID(gp[0].GetPeerID()), sim.GetPeerByID(gp[1].GetPeerID())}

	// The peer with the lesser ID offers, and the other answers.
	answerer := 0
	if strings.Compare(px[0].GetPeerID().String(), px[1].GetPeerID().String()) < 0 {
		answerer = 1
	}

	// Signal between the peers over a relay that can drop their signals.
	signalingID := "webrtc-signaling"
	var relay [2]*signalRelayEnd
	relay[0], relay[1] = newSignalRelay(
		signalingID,
		px[0].GetTestbed().Bus, px[1].GetTestbed().Bus,
		px[0].GetPeerPriv(), px[1].GetPeerPriv(),
		px[0].GetPeerID(), px[1].GetPeerID(),
	)
	for i, end := range relay {
		rel, err := px[i].GetTestbed().Bus.AddController(ctx, end, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer rel()
	}

	// Run a transport on each peer and hold a link from each to the other, as
	// both daemons do.
	tptConf := &webrtc.Config{SignalingId: signalingID, AllPeers: true, Verbose: true}
	var crash [2]func()
	for i := range px {
		crash[i] = iceNet.startTransport(ctx, t, le, px[i].GetTestbed().Bus, tptConf)
		_, ref, err := px[i].GetTestbed().Bus.AddDirective(
			link.NewEstablishLinkWithPeer(px[i].GetPeerID(), px[1-i].GetPeerID()),
			nil,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer ref.Release()
	}
	waitConnectivity(ctx, t, le, px[0], px[1], "initial", 30*time.Second)

	// Restart the answerer while its signaling path is down. It asks the
	// offerer for an offer, which replaces its session and offers again, and
	// the relay drops that offer.
	le.Infof("restarting the transport of p%d with its signaling path down", answerer)
	relay[answerer].down.Store(true)
	crash[answerer]()
	crash[answerer] = iceNet.startTransport(ctx, t, le, px[answerer].GetTestbed().Bus, tptConf)
	select {
	case <-relay[answerer].droppedOffers:
	case <-time.After(relinkBound):
		t.Fatal("the offerer did not offer to the restarted peer")
	}

	// The signaling path returns. The peers must link without another restart.
	relay[answerer].down.Store(false)
	waitConnectivity(ctx, t, le, px[0], px[1], "after-dropped-offer", relinkBound)
	for _, c := range crash {
		c()
	}
}
