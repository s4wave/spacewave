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

	// A reverse scan returns them backward, and Size counts them.
	it := tx.Iterate(ctx, nil, true, true)
	for i, key := range slices.Backward(keys) {
		if !it.Next() || string(it.Key()) != key {
			t.Fatalf("reverse scan diverged at %d: %v", i, it.Err())
		}
	}
	it.Close()
	if n, err := tx.Size(ctx); err != nil || n != uint64(len(m)) {
		t.Fatalf("size %d err %v, want %d", n, err, len(m))
	}
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

// checkSpace fails when two uses claim one page: a live value, a tree
// page, a log chunk, or the saved space against each other or against a
// free or pending run.
func checkSpace(t *testing.T, db *DB) {
	// Hold the writer lock so the space is stable.
	t.Helper()
	if err := db.w.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer db.w.unlock(nil)
	sp, st := db.w.sp, db.cur.Load()
	owner := make(map[uint64]string)
	claim := func(p uint64, who string, shared bool) {
		if prev, ok := owner[p]; ok && (!shared || prev != who) {
			t.Fatalf("page %d claimed by %s and %s", p, prev, who)
		}
		owner[p] = who
	}

	// Claim the pages under every stored value, the tree, the log chunks,
	// and the saved space.
	it := newIterator(db.p, st, nil, nil, false)
	for it.Next() {
		if v := it.cur.val; v.isRef {
			spans(v.ref.off, int(v.ref.n), func(p uint64, _ uint32) { claim(p, "value", true) })
		}
	}
	var walk func(p uint64)
	walk = func(p uint64) {
		claim(p, "tree", false)
		n, err := db.p.node(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	if st.root != 0 {
		walk(st.root)
	}
	for _, c := range sp.chunks {
		for _, p := range pageRange(c.start, c.n) {
			claim(p, "log", false)
		}
	}
	for _, p := range pageRange(st.sb.space.off/pageSize, pagesFor(int(st.sb.space.n))) {
		claim(p, "space", false)
	}

	// No claimed page may be free, pending, or past the end.
	for p, who := range owner {
		if p >= sp.end {
			t.Fatalf("%s page %d past the end %d", who, p, sp.end)
		}
		if r, ok := sp.before(p + 1); ok && p < r.end() {
			t.Fatalf("%s page %d in free run %+v", who, p, r)
		}
		for _, x := range slices.Concat(sp.values, sp.pages) {
			if p >= x.start && p < x.end() {
				t.Fatalf("%s page %d awaits release in %+v", who, p, x)
			}
		}
	}
}
