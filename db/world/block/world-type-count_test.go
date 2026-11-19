package world_block_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// TestWorldTypeCounts seeds Worlds of 10 and 10,000 objects of one type in
// each format and checks that reading the type counts reads the same number of
// blocks at both sizes, on an open Engine and on one opened for the read, and
// that each kind of type change moves the counts in its own commit.
func TestWorldTypeCounts(t *testing.T) {
	formats := []world_block.WorldFormat{
		world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV,
		world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH,
	}
	for _, format := range formats {
		t.Run(format.String(), func(t *testing.T) {
			ctx := t.Context()
			var warm, cold [2]uint64
			for i, n := range [2]int{10, 10000} {
				want := map[string]uint64{"count/a": uint64(n), "count/b": 3}
				tbed, eng := seedTypedWorld(t, ctx, format, n)
				warm[i] = readTypeCounts(t, ctx, eng, want)

				// Open a second Engine on the committed root and count the
				// reads of opening it and reading the counts.
				coldCtx, counter := block.WithReadCounter(ctx)
				coldEng, cleanup := openSeededEngine(coldCtx, t, tbed, eng.GetRootRef())
				readTypeCounts(t, coldCtx, coldEng, want)
				cold[i] = counter.Snapshot().BlockReadCount
				cleanup()

				if i == 0 {
					checkTypeCountChanges(t, ctx, eng, n)
				}
			}
			t.Logf("type count reads at 10 and 10000 objects: open Engine %v blocks, new Engine %v blocks", warm, cold)
			if warm[0] != warm[1] || cold[0] != cold[1] {
				t.Fatalf("type count reads grew with the World: open Engine %v blocks, new Engine %v blocks", warm, cold)
			}
		})
	}
}

// checkTypeCountChanges applies each kind of type change to a World seeded
// with n objects of count/a and checks the counts after each commit.
func checkTypeCountChanges(t *testing.T, ctx context.Context, eng *world_block.Engine, n int) {
	t.Helper()
	steps := []struct {
		name string
		op   func(ws world.WorldState) error
		want map[string]uint64
	}{
		{"rename", func(ws world.WorldState) error {
			obj, err := ws.RenameObject(ctx, typedObjectKey(0), "count/renamed", false)
			world.ReleaseObjectState(obj)
			return err
		}, map[string]uint64{"count/a": uint64(n), "count/b": 3}},
		{"retype", func(ws world.WorldState) error {
			return world_types.SetObjectType(ctx, ws, typedObjectKey(1), "count/b")
		}, map[string]uint64{"count/a": uint64(n - 1), "count/b": 4}},
		{"set again", func(ws world.WorldState) error {
			return world_types.SetObjectType(ctx, ws, typedObjectKey(1), "count/b")
		}, map[string]uint64{"count/a": uint64(n - 1), "count/b": 4}},
		{"delete object", func(ws world.WorldState) error {
			_, err := ws.DeleteObject(ctx, typedObjectKey(2))
			return err
		}, map[string]uint64{"count/a": uint64(n - 2), "count/b": 4}},
		{"delete edge", func(ws world.WorldState) error {
			return ws.DeleteGraphQuad(ctx, world.NewGraphQuadWithKeys(typedObjectKey(1), world_types.TypePred.String(), world_types.BuildTypeObjectKey("count/b"), ""))
		}, map[string]uint64{"count/a": uint64(n - 2), "count/b": 3}},
	}
	for _, step := range steps {
		// Read the counts inside the write transaction, then after its commit.
		err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
			if err := step.op(ws); err != nil {
				return err
			}
			expectTypeCounts(t, ctx, ws, step.want, step.name+" before commit")
			return nil
		})
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		readTypeCounts(t, ctx, eng, step.want)
	}
}

// seedTypedWorld returns a testbed and an Engine on a new World of format holding n objects
// of type count/a and 3 of count/b, written in transactions of at most 1000
// objects.
func seedTypedWorld(t *testing.T, ctx context.Context, format world_block.WorldFormat, n int) (*testbed.Testbed, *world_block.Engine) {
	t.Helper()

	// Write an empty World root of the format.
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tbed.Release)
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ocs.Release)
	btx, bcs := ocs.BuildTransaction(nil)
	bcs.SetBlock(world_block.NewWorld(format, false), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ocs.SetRootRef(rootRef)

	// Open the Engine and write the objects.
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	for start := 0; start < n+3; start += 1000 {
		err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
			for i := start; i < min(start+1000, n+3); i++ {
				typeID := "count/a"
				if i >= n {
					typeID = "count/b"
				}
				obj, err := ws.CreateObject(ctx, typedObjectKey(i), nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					return err
				}
				if err := world_types.SetObjectType(ctx, ws, typedObjectKey(i), typeID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return tbed, eng
}

// readTypeCounts checks the type counts in a read transaction of eng and
// returns the blocks the transaction and the read took, unless ctx already
// counts them.
func readTypeCounts(t *testing.T, ctx context.Context, eng *world_block.Engine, want map[string]uint64) uint64 {
	t.Helper()
	counter := block.ContextReadCounter(ctx)
	if counter == nil {
		ctx, counter = block.WithReadCounter(ctx)
	}
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	expectTypeCounts(t, ctx, tx, want, "committed")
	return counter.Snapshot().BlockReadCount
}

// expectTypeCounts checks the counts of the types in want.
func expectTypeCounts(t *testing.T, ctx context.Context, ws world.WorldState, want map[string]uint64, when string) {
	t.Helper()
	counts, err := ws.ListTypeCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for typeID, n := range want {
		if got := world.LookupTypeCount(counts, typeID); got != n {
			t.Fatalf("%s: %s count %d, want %d", when, typeID, got, n)
		}
	}
}

// typedObjectKey returns the key of typed object i.
func typedObjectKey(i int) string {
	return "count/object/" + strconv.Itoa(i)
}
