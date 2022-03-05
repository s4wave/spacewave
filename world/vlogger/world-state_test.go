package world_vlogger

import (
	"context"
	"testing"

	world_mock "github.com/aperturerobotics/hydra/world/mock"
	"github.com/aperturerobotics/hydra/world/testbed"
)

// TestWorldVlogger tests the world engine w/ vlogger enabled.
func TestWorldVlogger(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// basic sanity tests
	le, eng := tb.Logger, tb.Engine
	eng = NewEngine(le, eng)
	err = world_mock.TestWorldEngine_Basic(ctx, le, eng)
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}
