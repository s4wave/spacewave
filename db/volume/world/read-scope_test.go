package volume_world_test

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	"github.com/s4wave/spacewave/db/world/testbed"
)

// TestWorldVolumeReadScopeKeepsBackingStore exercises cursor reads through a
// Volume whose own metadata and block bytes are stored in a parent World.
func TestWorldVolumeReadScopeKeepsBackingStore(t *testing.T) {
	// Start a parent World testbed for nested volume storage.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Open a nested volume backed by the parent World engine.
	vol, err := volume_world.NewVolumeWithEngine(ctx, tb.Logger, tb.Bus, transform_all.BuildFactorySet(),
		&volume_world.Config{ObjectKey: "nested-volume"}, tb.Engine)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()

	// Store a block through the nested volume before opening its read scope.
	data := []byte("read through the nested volume and its parent World")
	ref, _, err := vol.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Retain the nested volume read scope while fetching its block cursor.
	scoped, release, err := vol.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Fetch the nested block through the retained backing store.
	_, cursor := block.NewTransaction(vol, nil, ref, nil)
	got, found, err := cursor.Fetch(block.WithReadOperationStore(ctx, scoped))
	if err != nil {
		t.Fatal(err)
	}

	// Check that the read scope preserved access to the parent World block.
	if !found || string(got) != string(data) {
		t.Fatalf("nested block = %q, found %v", got, found)
	}
}
