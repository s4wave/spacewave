package world_vlogger_test

import (
	"context"
	"testing"

	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/db/world/testbed"
)

// TestWorldVlogger tests the world engine w/ vlogger enabled.
func TestWorldVlogger(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx, testbed.WithWorldVerbose(true))
	if err != nil {
		t.Fatal(err.Error())
	}

	// basic sanity tests
	le, eng := tb.Logger, tb.Engine
	err = world_mock.TestWorldEngine_Basic(ctx, le, eng)
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
	t.Log("tests successful")
}
