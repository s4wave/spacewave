//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"testing"

	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
)

// openTest opens the database at path, failing the test on error.
func openTest(t *testing.T, path string, opts Options) *DB {
	t.Helper()
	db, err := Open(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// TestKvtx runs the shared store conformance suite.
func TestKvtx(t *testing.T) {
	db := openTest(t, filepath.Join(t.TempDir(), "kv.s4wave"), Options{})
	defer db.Close()
	if err := kvtest.TestAll(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

// TestModel commits random batches of small and large values, checks every
// key and scan against the model after each round, and reopens the file to
// check that the log replays to the same contents.
func TestModel(t *testing.T) {
	// Open the file and commit random rounds.
	path := filepath.Join(t.TempDir(), "model.s4wave")
	opts := Options{CacheBytes: 64 << 20}
	db := openTest(t, path, opts)
	r := rand.New(rand.NewPCG(1, 2))
	m := make(model)
	for round := range 6 {
		for range 25 {
			randomCommit(t, r, db, m, 3000)
		}
		m.check(t, db)

		// Reopen and check the replayed contents.
		if err := db.Close(); err != nil {
			t.Fatalf("round %d: close: %v", round, err)
		}
		db = openTest(t, path, opts)
		m.check(t, db)
		if got := contents(t, db); !maps.EqualFunc(got, m, bytes.Equal) {
			t.Fatalf("round %d: reopened contents differ: %d keys, want %d", round, len(got), len(m))
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestStats checks that the table's live and dead bytes follow the values
// written and replaced.
func TestStats(t *testing.T) {
	// Open a file counting family a apart.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "stats.s4wave"), Options{Families: [][]byte{[]byte("a/")}})
	defer db.Close()

	// Write two large values, then replace one.
	v := bytes.Repeat([]byte("x"), 4096)
	for _, k := range []string{"a/1", "b/1", "a/1"} {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte(k), v); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Two entries live, one dead, headers included. Three values written
	// and one freed in family a.
	s, err := db.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry := int64(valueEntryHeaderLen(3, 4096) + 4096)
	if s.KeyCount != 2 || s.LiveBytes != 2*entry || s.DeadBytes != entry {
		t.Fatalf("stats keys %d live %d dead %d", s.KeyCount, s.LiveBytes, s.DeadBytes)
	}
	if s.Families[0] != (Flow{Written: 2 * 4096, Freed: 4096}) || s.Families[1] != (Flow{Written: 4096}) {
		t.Fatalf("flows %+v", s.Families)
	}
	if len(s.Slots) != 1 || !s.Slots[0].Own {
		t.Fatalf("slots %+v", s.Slots)
	}
}

// TestWriteSuperFollowsPublisher checks monotonic generations and refusal of
// an older publication inside the sole writable handle.
func TestWriteSuperFollowsPublisher(t *testing.T) {
	// Publish a generation at a higher seq through the sole publisher.
	db := openTest(t, filepath.Join(t.TempDir(), "super.s4wave"), Options{})
	defer db.Close()
	planted := db.super
	planted.gen, planted.seq = db.super.gen+1, 50
	if err := db.writeSuper(planted); err != nil {
		t.Fatal(err)
	}

	// An older superblock is not written.
	older := planted
	older.seq = 40
	if err := db.writeSuper(older); err != nil {
		t.Fatal(err)
	}
	order, err := db.readSupers()
	if err != nil || order[0].gen != planted.gen || order[0].seq != 50 {
		t.Fatalf("after an older write: %+v, %v", order, err)
	}

	// A newer superblock follows the generation and becomes this handle's.
	newer := planted
	newer.seq = 60
	if err := db.writeSuper(newer); err != nil {
		t.Fatal(err)
	}
	if order, err = db.readSupers(); err != nil || order[0].gen != planted.gen+1 || order[0].seq != 60 || order[1].gen != planted.gen {
		t.Fatalf("after a newer write: %+v, %v", order, err)
	}
	if db.super.gen != planted.gen+1 {
		t.Fatalf("db.super gen %d, want %d", db.super.gen, planted.gen+1)
	}
}
