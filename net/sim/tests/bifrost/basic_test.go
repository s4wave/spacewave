package bifrost

import (
	"testing"

	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/sirupsen/logrus"
)

// TestBasic tests a simple connection between two peers on a LAN.
func TestBasic(t *testing.T) {
	// Log the simulation at debug level.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Build a graph of two peers on one LAN.
	g := graph.NewGraph()
	descrip := `p0 <-> [lan1] <-> p1`
	p0 := addPeer(t, g)
	p1 := addPeer(t, g)
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	// Simulate the graph and dial from p0 to p1.
	sim := initSimulator(
		t,
		ctx,
		le,
		g,
		simulate.WithVerbose(),
	)
	le.Infof("attempting to dial %v", descrip)
	if err := simulate.TestConnectivity(
		ctx,
		sim.GetPeerByID(p0.GetPeerID()),
		sim.GetPeerByID(p1.GetPeerID()),
	); err != nil {
		t.Fatal(err.Error())
	}
	le.Infof("successful connectivity test: %v", descrip)
}
