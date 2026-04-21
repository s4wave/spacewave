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
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	g := graph.NewGraph()
	addPeer(t, g)
	sim := initSimulator(t, ctx, le, g)
	_ = sim
}
