package store_kvtx_inmem

import (
	"context"
	"testing"
)

// TestTxSizeCountsVisibleKeys checks cardinality through overwrites and deletes.
func TestTxSizeCountsVisibleKeys(t *testing.T) {
	// Seed two committed keys so later writes can replace stored values.
	ctx := context.Background()
	store := NewStore()
	seed, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Discard()
	for _, key := range []string{"a", "b"} {
		if err := seed.Set(ctx, []byte(key), []byte("initial")); err != nil {
			t.Fatal(err)
		}
	}
	if err := seed.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Apply changes to one transaction and check its visible key count each time.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, step := range []struct {
		// name describes the transition under test.
		name string
		// key identifies the entry to change.
		key string
		// remove chooses deletion instead of replacement.
		remove bool
		// want is the number of visible keys after the change.
		want uint64
	}{
		{name: "overwrite stored", key: "a", want: 2},
		{name: "overwrite stored again", key: "a", want: 2},
		{name: "add new", key: "c", want: 3},
		{name: "overwrite new", key: "c", want: 3},
		{name: "delete stored", key: "a", remove: true, want: 2},
		{name: "restore stored", key: "a", want: 3},
		{name: "delete new", key: "c", remove: true, want: 2},
		{name: "delete new again", key: "c", remove: true, want: 2},
		{name: "delete missing", key: "missing", remove: true, want: 2},
		{name: "delete second stored", key: "b", remove: true, want: 1},
		{name: "delete last", key: "a", remove: true, want: 0},
	} {
		// Change the entry through the production transaction interface.
		switch step.remove {
		case true:
			err = tx.Delete(ctx, []byte(step.key))
		case false:
			err = tx.Set(ctx, []byte(step.key), []byte(step.name))
		}
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}

		// An overwrite changes a value, never the number of visible keys.
		size, err := tx.Size(ctx)
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if size != step.want {
			t.Fatalf("%s: size = %d, want %d", step.name, size, step.want)
		}
	}

	// The committed store agrees with the transaction's final empty view.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	size, err := read.Size(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("committed size = %d, want 0", size)
	}
}
