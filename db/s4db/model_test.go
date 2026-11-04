package s4db

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
// free or pending run. It also fails when a page below the end is none of
// these, reserved, or in the open extent, which leaks it.
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

	// Claim the pages under every stored value, counting their live bytes.
	live := make(map[uint64]uint16)
	it := newIterator(db.p, st, nil, nil, false)
	for it.Next() {
		if v := it.cur.val; v.isRef {
			spans(v.ref.off, int(v.ref.n), func(p uint64, b uint16) {
				claim(p, "value", true)
				live[p] += b
			})
		}
	}

	// The space counts the same live bytes.
	counted := make(map[uint64]uint16)
	for p, b := range sp.live {
		if b != 0 {
			counted[uint64(p)] = b
		}
	}
	if !maps.Equal(live, counted) || sp.livePages != int64(len(counted)) {
		for p, b := range live {
			if counted[p] != b {
				t.Errorf("value page %d holds %d live bytes, space counts %d", p, b, counted[p])
			}
		}
		for p, b := range counted {
			if _, ok := live[p]; !ok {
				t.Errorf("space counts %d live bytes on page %d, which holds none", b, p)
			}
		}
		if sp.livePages != int64(len(counted)) {
			t.Errorf("space counts %d live pages, %d hold live bytes", sp.livePages, len(counted))
		}
		t.FailNow()
	}

	// Claim the tree, the log chunks, and the saved space.
	var walk func(p uint64)
	walk = func(p uint64) {
		claim(p, "tree", false)
		n, err := db.p.node(p)
		if err != nil {
			t.Fatal(err)
		}
		if !n.leaf {
			for i := range n.count() {
				walk(n.kid(i))
			}
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

	// Mark the pages free, awaiting release, or reserved.
	held := make([]bool, sp.end)
	mark := func(r run) {
		for p := r.start; p < r.end() && p < sp.end; p++ {
			held[p] = true
		}
	}
	sp.free.Scan(func(start, n uint64) bool {
		mark(run{start: start, n: n})
		return true
	})
	for _, x := range slices.Concat(sp.values, sp.pages) {
		mark(x.run)
	}
	for _, r := range sp.reserved {
		mark(r)
	}

	// Every other page past the superblocks is claimed or in the open
	// extent.
	var leaked []uint64
	for p := uint64(firstPage); p < sp.end; p++ {
		if _, ok := owner[p]; !ok && !held[p] && !sp.inOpen(p) {
			leaked = append(leaked, p)
		}
	}
	if len(leaked) != 0 {
		t.Fatalf("pages %v are neither used, free, pending, nor reserved", leaked)
	}
}

// TestDeepTreeGets reads every key after each commit to a tree whose long
// shared key prefix makes inner pages narrow and deep. Checkpoints rewrite
// inner pages often, and each must route a key equal to a separator to the
// child that holds it.
func TestDeepTreeGets(t *testing.T) {
	// Open a database that checkpoints every few commits.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "a.s4wave"), Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10})
	r := rand.New(rand.NewPCG(1, 2))
	prefix := strings.Repeat("p", 120)
	m := make(model)
	for n := range 300 {
		// Set or delete a random batch.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for range 1 + r.IntN(150) {
			k := fmt.Sprintf("%s/%07d", prefix, r.IntN(30000))
			if r.IntN(3) == 0 {
				delete(m, k)
				err = tx.Delete(ctx, []byte(k))
			} else {
				m[k] = []byte(k[len(prefix):])
				err = tx.Set(ctx, []byte(k), m[k])
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		// Read every key back.
		rtx, err := db.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		for k := range m {
			if _, ok, err := rtx.Get(ctx, []byte(k)); err != nil || !ok {
				t.Fatalf("commit %d: get %s: found %v err %v", n, k[len(prefix):], ok, err)
			}
		}
		rtx.Discard()
	}
}

// TestGetBatchPrefetch checks that a cold GetBatch answers present and absent
// keys and leaves every page on each key's path cached.
func TestGetBatchPrefetch(t *testing.T) {
	// Build a deep tree across checkpoints.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "a.s4wave"), Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10})
	defer db.Close()
	prefix := strings.Repeat("p", 120)
	for n := range 40 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := range 200 {
			k := fmt.Sprintf("%s/%07d", prefix, 2*(n*200+i))
			if err := tx.Set(ctx, []byte(k), []byte(k[len(prefix):])); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// List even keys, which are present, and odd keys, which are absent.
	var keys [][]byte
	for i := 0; i < 16000; i += 37 {
		keys = append(keys, fmt.Appendf(nil, "%s/%07d", prefix, i))
	}

	// Read them from a cold cache.
	db.p.cache.clear()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	values, found, err := tx.(kvtx.BatchTxOps).GetBatch(ctx, keys)
	if err != nil {
		t.Fatal(err)
	}
	for i, key := range keys {
		want := i*37%2 == 0
		if found[i] != want || (want && !bytes.Equal(values[i], key[len(prefix):])) {
			t.Fatalf("key %s: found %v value %q", key[len(prefix):], found[i], values[i])
		}
	}

	// Prefetch the keys through a second pager with a cold cache, whose
	// reads each take long enough to overlap with the reads beside them.
	root := tx.(*Tx).st.root
	if root == 0 {
		t.Fatal("no checkpointed tree")
	}
	r := &overlapReader{r: db.s}
	p := &pager{r: r, cache: newCache(64 << 20)}
	if err := p.prefetch(ctx, root, keys); err != nil {
		t.Fatal(err)
	}
	if r.most.Load() < 2 {
		t.Fatalf("prefetch kept at most %d reads in flight", r.most.Load())
	}

	// Descend to each key through the cache alone.
	for _, key := range keys {
		for page := root; ; {
			n := p.cache.get(page)
			if n == nil {
				t.Fatalf("key %s: page %d not cached", key[len(prefix):], page)
			}
			if n.leaf {
				break
			}
			page = n.kid(n.childIndex(key))
		}
	}
}

// TestCommitPrefetch checks that a commit on a cold cache reads the index
// paths of its changes together instead of one path at a time, and reads each
// page once even when the cache cannot hold the paths and the overlay filter
// claims every key, so the lookups under the writer lock read nothing more.
func TestCommitPrefetch(t *testing.T) {
	// Build a deep tree across checkpoints.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "a.s4wave")
	opts := Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10, RelocateBudget: -1}
	db := openTest(t, path, opts)
	prefix := strings.Repeat("p", 120)
	for n := range 40 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := range 200 {
			k := fmt.Sprintf("%s/%07d", prefix, n*200+i)
			if err := tx.Set(ctx, []byte(k), []byte(k[len(prefix):])); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Reopen it so its index is on disk, with no checkpoint to read pages
	// after the commit, and let the warm-up finish before the test empties
	// the cache.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, Options{CheckpointMin: 1 << 30, CheckpointMax: 1 << 30, RelocateBudget: -1})
	defer db.Close()
	if wait, _ := db.warmer.SetRoutine(nil); wait != nil {
		<-wait
	}

	// Count the distinct pages on the paths of scattered keys, as a
	// prefetch through a cold cache reads each once.
	var keys [][]byte
	for i := 0; i < 8000; i += 97 {
		keys = append(keys, fmt.Appendf(nil, "%s/%07d", prefix, i))
	}
	st := db.cur.Load()
	paths := &pager{r: db.s, cache: newCache(64 << 20)}
	if err := paths.prefetch(ctx, st.root, keys); err != nil {
		t.Fatal(err)
	}
	if paths.reads.Load() <= int64(len(keys)) {
		t.Fatalf("paths hold %d pages for %d keys, want a deeper tree", paths.reads.Load(), len(keys))
	}

	// Delete and replace the keys through a cache of one page per shard,
	// whose reads each take long enough to overlap with the reads beside
	// them, with every key in the overlay filter.
	r := &overlapReader{r: db.s}
	db.p.r = r
	db.p.cache = newCache(pageSize)
	for _, k := range keys {
		st.filter.add(k)
	}
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for j, k := range keys {
		if j%2 == 0 {
			err = tx.Delete(ctx, k)
		} else {
			err = tx.Set(ctx, k, []byte("new"))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	reads := db.p.reads.Load()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if r.most.Load() < 2 {
		t.Fatalf("commit kept at most %d reads in flight", r.most.Load())
	}
	if got, want := db.p.reads.Load()-reads, paths.reads.Load(); got > want {
		t.Fatalf("commit read %d pages, want at most the %d on its paths", got, want)
	}

	// Check the commit's result.
	rtx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	for j, k := range keys {
		v, found, err := rtx.Get(ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		if found != (j%2 != 0) || (found && string(v) != "new") {
			t.Fatalf("key %s: found %v value %q", k[len(prefix):], found, v)
		}
	}
}

// overlapReader records the most reads in flight at once.
type overlapReader struct {
	r          io.ReaderAt
	open, most atomic.Int32
}

// ReadAt reads from the underlying file and holds the read open briefly.
func (o *overlapReader) ReadAt(b []byte, off int64) (int, error) {
	// Count the read in flight and raise the most seen.
	n := o.open.Add(1)
	defer o.open.Add(-1)
	for m := o.most.Load(); n > m && !o.most.CompareAndSwap(m, n); m = o.most.Load() {
	}

	// Hold the read open so concurrent reads overlap it.
	time.Sleep(time.Millisecond)
	return o.r.ReadAt(b, off)
}
