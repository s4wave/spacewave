package world_block

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestWorldState_Basic performs a simple test of operations against world.
func TestWorldState_Basic(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := BuildMockWorldState(ctx, ocs)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, err = BuildMockObject(ctx, ws, "")
	if err != nil {
		t.Fatal(err.Error())
	}

	err = ws.Commit()
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs.SetRootRef(ws.GetRootRef())

	// success
}
