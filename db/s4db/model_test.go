package s4db

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

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

	// A reverse scan returns them backward, each key unchanged after the
	// iterator moves past its block, and Size counts them.
	it := tx.Iterate(ctx, nil, true, true)
	var prev []byte
	for i, key := range slices.Backward(keys) {
		if !it.Next() || string(it.Key()) != key {
			t.Fatalf("reverse scan diverged at %d: got %q, %v", i, it.Key(), it.Err())
		}
		if prev != nil && string(prev) != keys[i+1] {
			t.Fatalf("key %d changed to %q after Next", i+1, prev)
		}
		prev = it.Key()
	}
	it.Close()

	// A forward seek just past a key lands on the next one, wherever in its
	// block the key sits.
	it = tx.Iterate(ctx, nil, true, false)
	for i := 0; i < len(keys); i += 7 {
		if err := it.Seek(append([]byte(keys[i]), 0)); err != nil {
			t.Fatal(err)
		}
		if i+1 < len(keys) && (!it.Valid() || string(it.Key()) != keys[i+1]) {
			t.Fatalf("seek past %q landed on %q, want %q", keys[i], it.Key(), keys[i+1])
		}
		if i+1 == len(keys) && it.Valid() {
			t.Fatalf("seek past the last key landed on %q", it.Key())
		}
	}
	it.Close()
	if n, err := tx.Size(ctx); err != nil || n != uint64(len(m)) {
		t.Fatalf("size %d err %v, want %d", n, err, len(m))
	}

	// The table's tally matches a walk of its entries.
	checkTally(t, db.cur.Load().m.table)
}

// checkTally recomputes the tally of table from its entries and compares.
func checkTally(t *testing.T, table *chunkTable) {
	// Walk the entries.
	t.Helper()
	var want chunkTable
	for c, e := range table.all() {
		want.tally(e, 1)
		if e.kind == chunkFree && c != 0 && c < table.lowFree {
			t.Fatalf("chunk %d is free below the free hint %d", c, table.lowFree)
		}
	}

	// Compare the counters.
	if want.kinds != table.kinds || want.live != table.live || want.used != table.used {
		t.Fatalf("tally kinds %v live %d used %d, walk gives %v live %d used %d", table.kinds, table.live, table.used, want.kinds, want.live, want.used)
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
