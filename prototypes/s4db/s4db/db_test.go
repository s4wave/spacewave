//go:build darwin || linux

package s4db

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
)

// openTest opens a database in a temporary directory.
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
	// Open a write transaction.
	t.Helper()
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Set or delete up to 64 random keys.
	for range 1 + r.IntN(64) {
		k := fmt.Sprintf("k/%06d", r.IntN(keys))
		if r.IntN(4) == 0 {
			delete(m, k)
			if err := tx.Delete(ctx, []byte(k)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		v := make([]byte, []int{8, 100, 600, 3000, 20000}[r.IntN(5)])
		for i := range v {
			v[i] = byte(r.Uint32())
		}
		m[k] = v
		if err := tx.Set(ctx, []byte(k), v); err != nil {
			t.Fatal(err)
		}
	}

	// Commit durably or ordered at random.
	if r.IntN(2) == 0 {
		err = tx.(kvtx.OrderedCommitTx).CommitOrdered(ctx)
	} else {
		err = tx.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// TestModel checks random commits against a map across checkpoints,
// compaction, and reopening.
func TestModel(t *testing.T) {
	// Commit, check, compact, and reopen in rounds.
	path := filepath.Join(t.TempDir(), "model.s4wave")
	opts := Options{CheckpointMin: 64 << 10, CheckpointMax: 256 << 10}
	r := rand.New(rand.NewPCG(1, 2))
	m := make(model)
	db := openTest(t, path, opts)
	for round := range 8 {
		for range 200 {
			randomCommit(t, r, db, m, 4000)
		}
		m.check(t, db)
		checkSpace(t, db)
		if round%3 == 2 {
			if err := db.Compact(); err != nil {
				t.Fatal(err)
			}
			m.check(t, db)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openTest(t, path, opts)
		m.check(t, db)
	}

	// Delete everything and compact.
	ctx := context.Background()
	tx, _ := db.NewTransaction(ctx, true)
	for k := range m {
		_ = tx.Delete(ctx, []byte(k))
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	clear(m)

	// The emptied file shrinks to a few pages.
	if err := db.Compact(); err != nil {
		t.Fatal(err)
	}
	m.check(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	used, size := usage(t, path)
	t.Logf("empty file: %d bytes long, %d allocated", size, used)
	if used > 256<<10 || size > 2<<20 {
		t.Fatalf("empty database is %d bytes long with %d allocated", size, used)
	}
}

// TestTornTail cuts the file inside the last record and checks that reopening
// keeps every earlier commit.
func TestTornTail(t *testing.T) {
	// Commit a history.
	dir := t.TempDir()
	path := filepath.Join(dir, "torn.s4wave")
	r := rand.New(rand.NewPCG(3, 4))
	m := make(model)
	db := openTest(t, path, Options{})
	for range 50 {
		randomCommit(t, r, db, m, 500)
	}

	// Commit once more, noting where the last record lies.
	before := maps.Clone(m)
	db.mtx.Lock()
	pos := db.st.pos
	db.mtx.Unlock()
	randomCommit(t, r, db, m, 500)
	db.mtx.Lock()
	end := db.st.pos
	db.mtx.Unlock()

	// Copy the file as a crash would leave it: the last record half written.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	clear(b[pos+(end-pos)/2 : end])
	crashed := filepath.Join(dir, "crashed.s4wave")
	if err := os.WriteFile(crashed, b, 0o600); err != nil {
		t.Fatal(err)
	}

	// Reopening the copy keeps every earlier commit.
	_ = db.Close()
	db = openTest(t, crashed, Options{})
	defer db.Close()
	before.check(t, db)

	// The log continues after the torn record.
	randomCommit(t, r, db, before, 500)
	before.check(t, db)
}

// TestTwoHandles writes through two handles of one file, as two processes
// would, and checks each sees the other's commits.
func TestTwoHandles(t *testing.T) {
	// Commit through the first handle.
	path := filepath.Join(t.TempDir(), "two.s4wave")
	r := rand.New(rand.NewPCG(5, 6))
	m := make(model)
	a := openTest(t, path, Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10})
	defer a.Close()
	for range 20 {
		randomCommit(t, r, a, m, 300)
	}

	// The second handle sees the first handle's commits on open.
	b := openTest(t, path, Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10})
	defer b.Close()
	m.check(t, b)

	// Alternate commits between the handles and flush.
	for i := range 200 {
		db := a
		if i%3 == 0 {
			db = b
		}
		randomCommit(t, r, db, m, 300)
	}
	if err := b.WaitDurable(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Each writer catches up on its next write; reads see the published
	// state once the tail loop applies it.
	for _, db := range []*DB{a, b} {
		tx, _ := db.NewTransaction(context.Background(), true)
		tx.Discard()
		m.check(t, db)
	}
}

// checkSpace fails when a live value lies on a free or pending page.
func checkSpace(t *testing.T, db *DB) {
	// Hold the writer lock so the space is stable.
	t.Helper()
	if err := db.lockWriter(); err != nil {
		t.Fatal(err)
	}
	defer db.unlockWriter()

	// Check each page under every stored value.
	sp := db.sp
	it := newIterator(db, db.st, nil, nil, false)
	for it.Next() {
		if !it.cur.isRef {
			continue
		}
		spans(it.cur.ref.off, int(it.cur.ref.n), func(p uint64, _ uint32) {
			if p >= sp.end {
				t.Fatalf("key %s: page %d past the end %d", it.Key(), p, sp.end)
			}
			if s, n, ok := sp.before(p + 1); ok && p < s+n {
				t.Fatalf("key %s: page %d in free run %d+%d", it.Key(), p, s, n)
			}
			for _, x := range slices.Concat(sp.values, sp.pages) {
				if p >= x.start && p < x.start+x.n {
					t.Fatalf("key %s: page %d awaits release in %+v", it.Key(), p, x)
				}
			}
		})
	}
}
