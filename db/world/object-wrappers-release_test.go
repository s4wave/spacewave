package world_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_vlogger "github.com/s4wave/spacewave/db/world/vlogger"
)

// TestObjectWrappersRelease verifies that wrapping an interface cannot hide its
// underlying resource's optional Release method from ReleaseObjectState.
func TestObjectWrappersRelease(t *testing.T) {
	for name, wrap := range map[string]func(world.ObjectState) world.ObjectState{
		"logging": func(obj world.ObjectState) world.ObjectState {
			return world_vlogger.NewObjectState(nil, obj)
		},
		"block transaction": func(obj world.ObjectState) world.ObjectState {
			return world_block.NewTxObjectState(nil, "key", obj)
		},
		"batch transaction": func(obj world.ObjectState) world.ObjectState {
			return world_block_tx.NewObjectState(nil, "key", obj)
		},
	} {
		t.Run(name, func(t *testing.T) {
			releases := 0
			obj := wrap(&releaseCountingObjectState{releases: &releases})
			world.ReleaseObjectState(obj)
			if releases != 1 {
				t.Fatalf("wrapper released underlying handle %d times, want 1", releases)
			}
		})
	}
}
