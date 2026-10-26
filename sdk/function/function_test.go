package s4wave_function_test

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
)

// TestSetPureFunction creates a function, replaces it in place, and refuses an
// object of another type.
func TestSetPureFunction(t *testing.T) {
	// Start a World.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	exec := func(fn func(ctx context.Context, tx world.WorldState) error) error {
		return world.ExecTransaction(ctx, tb.Engine, true, fn)
	}
	set := func(key, source string) error {
		return exec(func(ctx context.Context, tx world.WorldState) error {
			return s4wave_function.SetPureFunction(ctx, tx, key, &s4wave_function.PureFunction{
				Source:     source,
				Module:     source,
				InputType:  "in",
				OutputType: "out",
			})
		})
	}

	// read returns the function at key and its object's revision.
	read := func(key string) (*s4wave_function.PureFunction, uint64) {
		// Read both in one transaction.
		t.Helper()
		var fn *s4wave_function.PureFunction
		var rev uint64
		err := exec(func(ctx context.Context, tx world.WorldState) error {
			// Read the function.
			var err error
			fn, err = s4wave_function.ReadPureFunction(ctx, tx, key)
			if err != nil {
				return err
			}

			// Read its object's revision.
			obj, _, err := tx.GetObject(ctx, key)
			defer world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			_, rev, err = obj.GetRootRef(ctx)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return fn, rev
	}

	// The first set creates the function.
	if err := set("functions/a", "one"); err != nil {
		t.Fatal(err)
	}
	fn, rev := read("functions/a")
	if fn.GetSource() != "one" {
		t.Fatalf("created function: %v", fn)
	}

	// The second set replaces it in place at a later revision.
	if err := set("functions/a", "two"); err != nil {
		t.Fatal(err)
	}
	fn, next := read("functions/a")
	if fn.GetSource() != "two" || next <= rev {
		t.Fatalf("replaced function: %v at rev %d after %d", fn, next, rev)
	}

	// An object of another type is refused.
	err := exec(func(ctx context.Context, tx world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, tx, "other", func(bcs *block.Cursor) error {
			bcs.SetBlock(&s4wave_function.PureFunction{}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, tx, "other", "test/other")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := set("other", "three"); err == nil {
		t.Fatal("set over another type succeeded")
	}
}
