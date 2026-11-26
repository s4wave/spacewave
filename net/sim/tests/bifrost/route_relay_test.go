//go:build bifrost_relay

package bifrost

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/sirupsen/logrus"
)

// TestRouteRelay tests a simple one-hop relay routing scenario.
// TODO: implement circuit/ to pass this test
func TestRouteRelay(t *testing.T) {
	// Scope the relay simulation to the test lifetime.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	// Record the relay simulation at debug level.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build the three-peer topology with a relay joining two LANs.
	g := graph.NewGraph()
	descrip := `p0 <-> [lan1] <-> p1 <-> [lan2] <-> p2`
	p0 := addPeer(t, g)
	p1 := addPeer(t, g)
	p2 := addPeer(t, g)
	lan1 := graph.AddLAN(g)
	lan2 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)
	lan2.AddPeer(g, p1)
	lan2.AddPeer(g, p2)

	// Start the graph before probing the relayed connection.
	sim := initSimulator(t, ctx, le, g)

	// Verify connectivity from the first LAN through the relay.
	le.Infof("attempting to dial %v", descrip)
	if err := sim.GetPeerByID(p0.GetPeerID()).TestConnectivity(ctx, sim.GetPeerByID(p2.GetPeerID())); err != nil {
		t.Fatal(err.Error())
	}

	// Report completion of the relayed connectivity check.
	le.Info("tests successful")
	_ = sim
}
