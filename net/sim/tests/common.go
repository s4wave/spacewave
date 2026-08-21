package tests

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/sirupsen/logrus"
)

// AddPeer generates a keypair and adds a peer to the graph, failing the
// test on error.
func AddPeer(t *testing.T, g *graph.Graph) *graph.Peer {
	ctx := context.Background()

	// Add a generated graph peer for the test.
	p, err := graph.GenerateAddPeer(ctx, g)
	if err != nil {
		t.Fatal(err.Error())
	}
	return p
}

// InitSimulator constructs the simulator for the graph, failing the test on
// error.
func InitSimulator(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
	g *graph.Graph,
	opts ...simulate.SimulatorOption,
) *simulate.Simulator {
	sim, err := simulate.NewSimulator(ctx, le, g, opts...)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("simulator startup complete")
	return sim
}
