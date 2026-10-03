package simulate

import (
	"testing"

	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/sirupsen/logrus"
)

// TestSimpleSimulate tests a simple simulation.
func TestSimpleSimulate(t *testing.T) {
	// Prepare the simulation context and debug logger.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Create the network graph for the simulated peers.
	g := graph.NewGraph()

	// Prepare a peer generator bound to the simulation graph.
	addPeer := func() *graph.Peer {
		p, err := graph.GenerateAddPeer(ctx, g)
		if err != nil {
			t.Fatal(err.Error())
		}
		return p
	}

	// Place the first two peers on the same simulated LAN.
	p0 := addPeer()
	p1 := addPeer()
	lan1 := graph.AddLAN(g)
	lan1.AddPeer(g, p0)
	lan1.AddPeer(g, p1)

	// Connect the third peer through a second simulated LAN.
	p2 := addPeer()
	lan2 := graph.AddLAN(g)
	lan2.AddPeer(g, p2)
	lan2.AddConnectionToLAN(g, lan1)

	// Start the simulator with the connected peer graph.
	sim, err := NewSimulator(ctx, le, g)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("simulator startup complete")

	// Prepare a connectivity assertion for pairs of simulated peers.
	assertConnectivity := func(p0, p1 *graph.Peer) {
		px0 := sim.GetPeerByID(p0.GetPeerID())
		px1 := sim.GetPeerByID(p1.GetPeerID())
		if err := TestConnectivity(ctx, px0, px1); err != nil {
			t.Fatal(err.Error())
		}
		le.Infof(
			"successful connectivity test between %s and %s",
			p0.GetPeerID().String(),
			p1.GetPeerID().String(),
		)
	}

	// Verify connectivity within one LAN and across connected LANs.
	assertConnectivity(p0, p1)
	assertConnectivity(p1, p0)
	assertConnectivity(p2, p0)

	// Report completion of the simulated connectivity checks.
	le.Info("tests successful")
	_ = sim
}
