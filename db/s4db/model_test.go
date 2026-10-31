package s4db

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
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

// model is the expected contents of a database.
type model map[string][]byte

// check compares every key and a full scan of db against m.
func (m model) check(t *testing.T, db *DB) {
	// Read every key in a read transaction.
	t.Helper()
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	keys := make([]string, 0, len(m))
	for k, v := range m {
		keys = append(keys, k)
		got, ok, err := tx.Get(ctx, []byte(k))
		if err != nil || !ok || !bytes.Equal(got, v) {
			t.Fatalf("get %x: found %v err %v, len %d want %d", k, ok, err, len(got), len(v))
		}
	}

	// A forward scan returns the keys in order.
	slices.Sort(keys)
	var scanned []string
	if err := tx.ScanPrefixKeys(ctx, nil, func(k []byte) error {
		scanned = append(scanned, string(k))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keys, scanned) {
		t.Fatalf("scan returned %d keys, want %d", len(scanned), len(keys))
	}

	// A reverse scan returns them backward, and Size counts them.
	it := tx.Iterate(ctx, nil, true, true)
	for i, key := range slices.Backward(keys) {
		if !it.Next() || string(it.Key()) != key {
			t.Fatalf("reverse scan diverged at %d: got %q, %v", i, it.Key(), it.Err())
		}
	}
	it.Close()
	if n, err := tx.Size(ctx); err != nil || n != uint64(len(m)) {
		t.Fatalf("size %d err %v, want %d", n, err, len(m))
	}
}

// contents reads every key of db into a model.
func contents(t *testing.T, db *DB) model {
	// Scan every key in one read transaction.
	t.Helper()
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	m := make(model)
	if err := tx.ScanPrefix(ctx, nil, func(k, v []byte) error {
		m[string(k)] = bytes.Clone(v)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return m
}

// randomCommit applies a random batch to db and m.
func randomCommit(t *testing.T, r *rand.Rand, db *DB, m model, keys int) {
	t.Helper()
	if _, err := commitRandom(r, db, m, keys); err != nil {
		t.Fatal(err)
	}
}

// commitRandom applies a random batch to m and commits it to db, durably
// or ordered at random, reporting whether the commit was durable.
func commitRandom(r *rand.Rand, db *DB, m model, keys int) (bool, error) {
	// Open a write transaction.
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}

	// Set or delete up to 64 random keys.
	for range 1 + r.IntN(64) {
		k := fmt.Sprintf("k/%06d", r.IntN(keys))
		if r.IntN(4) == 0 {
			delete(m, k)
			if err := tx.Delete(ctx, []byte(k)); err != nil {
				tx.Discard()
				return false, err
			}
			continue
		}
		v := make([]byte, []int{8, 100, 600, 3000, 20000}[r.IntN(5)])
		for i := range v {
			v[i] = byte(r.Uint32())
		}
		m[k] = v
		if err := tx.Set(ctx, []byte(k), v); err != nil {
			tx.Discard()
			return false, err
		}
	}

	// Commit durably or ordered at random.
	if r.IntN(2) == 0 {
		return false, tx.(kvtx.OrderedCommitTx).CommitOrdered(ctx)
	}
	return true, tx.Commit(ctx)
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

	// Two entries live, one dead, headers included; three values written and
	// one freed in family a.
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
