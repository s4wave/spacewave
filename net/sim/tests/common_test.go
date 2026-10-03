package tests

import (
	"testing"

	"github.com/s4wave/spacewave/net/sim/graph"
	"github.com/sirupsen/logrus"
)

var (
	addPeer       = AddPeer
	initSimulator = InitSimulator
)

func TestInitSimulator(t *testing.T) {
	// Prepare the simulator test context and debug logger.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Initialize the simulator with a graph containing one peer.
	g := graph.NewGraph()
	addPeer(t, g)
	sim := initSimulator(t, ctx, le, g)
	_ = sim
}
