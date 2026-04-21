package tests

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/s4wave/spacewave/net/sim/simulate"
	"github.com/sirupsen/logrus"
)

func AddPeer(t *testing.T, g *graph.Graph) *graph.Peer {
	ctx := context.Background()
	p, err := graph.GenerateAddPeer(ctx, g)
	if err != nil {
		t.Fatal(err.Error())
	}
	return p
}

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
