package world_block

import (
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
)

// TestReadSetTracksOperationReads checks that a write applying an operation
// records the object the operation reads and changes, not the whole World.
func TestReadSetTracksOperationReads(t *testing.T) {
	// Create the object the operation changes.
	ctx := t.Context()
	e := newRetirementTestEngine(t, ctx)
	createWatchTestObject(t, ctx, world.NewEngineWorldState(e, true), "op/target")

	// Apply the operation in a write tracked by a ReadSet.
	tx, err := e.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	reads := world.NewReadSet()
	if _, _, err := reads.Track(tx).ApplyWorldOp(ctx, world_mock.NewMockWorldOp("op/target", "changed"), ""); err != nil {
		t.Fatal(err)
	}

	// A change to the operation's object touches the reads; another does not.
	if !reads.TouchedStrict(&world.ChangeSet{Keys: []string{"op/target"}}) {
		t.Error("a change to the operation's object missed the reads")
	}
	if reads.TouchedStrict(&world.ChangeSet{Keys: []string{"op/other"}}) {
		t.Error("a change to another object touched the reads")
	}
}
