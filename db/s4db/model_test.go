package s4db

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// checkSpace fails when the chunk table disagrees with the file: two uses
// claim one page, a use lies in a free or pending run or a chunk of another
// kind, a chunk's live count differs from the values and pages in it, a
// value's header does not name its key, or a chunk below the end is used by
// nothing, which leaks it. It also fails when the space rebuilt from the
// file differs from the writer's.
func checkSpace(t *testing.T, db *DB) {
	// Hold the writer lock so the space is stable.
	t.Helper()
	if err := db.w.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer db.w.unlock(nil)
	sp, st := db.w.sp, db.cur.Load()
	owner := make(map[uint64]string)
	claim := func(p uint64, who string) {
		if prev, ok := owner[p]; ok {
			t.Fatalf("page %d claimed by %s and %s", p, prev, who)
		}
		owner[p] = who
	}

	// Count the live bytes of every stored value by chunk, and check that
	// its header precedes it.
	live := make(map[uint64]uint32)
	it := newIterator(db.p, st, nil, nil, false)
	for it.Next() {
		v := it.cur.val
		if !v.isRef {
			continue
		}
		chunkSpans(v.ref.off, int(v.ref.n), func(c uint64, b uint32) {
			if k := sp.table[c].kind; k != chunkValue && k != chunkTail {
				t.Fatalf("value of %q in chunk %d of kind %d", it.Key(), c, k)
			}
			live[c] += b
		})
		want := appendValueHeader(nil, it.Key(), make([]byte, v.ref.n))
		got := make([]byte, len(want))
		if _, err := db.s.ReadAt(got, fileOff(v.ref.off-uint64(len(want)))); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("value of %q has header %x, want %x", it.Key(), got, want)
		}
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}

	// Claim the tree, the log chunks, and the saved space.
	var walk func(p uint64)
	walk = func(p uint64) {
		claim(p, "tree")
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
	for _, c := range sp.log {
		for _, p := range pageRange(c.start, c.n) {
			claim(p, "log")
		}
	}
	for _, p := range pageRange(st.sb.space.off/pageSize, pagesFor(int(st.sb.space.n))) {
		claim(p, "space")
	}

	// No claimed page may be pending or in a chunk not holding it.
	pend := make(map[uint64]bool)
	for _, x := range slices.Concat(sp.values, sp.pages) {
		for _, p := range pageRange(x.start, x.n) {
			pend[p] = true
		}
	}
	for p, who := range owner {
		c := p / chunkPages
		if c >= uint64(len(sp.table)) {
			t.Fatalf("%s page %d past the end %d", who, p, sp.end())
		}
		if pend[p] {
			t.Fatalf("%s page %d awaits release", who, p)
		}
		e := &sp.table[c]
		switch {
		case who == "log" && e.kind != chunkLog:
			t.Fatalf("log page %d in chunk of kind %d", p, e.kind)
		case who != "log" && (e.kind != chunkIndex || e.used[p%chunkPages/64]&(1<<(p%64)) == 0):
			t.Fatalf("%s page %d not used in its chunk of kind %d", who, p, e.kind)
		}
	}
	for _, r := range sp.reserved {
		for _, p := range pageRange(r.start, r.n) {
			claim(p, "reserved")
		}
	}

	// The file ends in a chunk that holds something.
	if l := len(sp.table); l == 0 || sp.table[l-1].kind == chunkFree || sp.table[l-1].kind == chunkHeld {
		t.Fatalf("last of %d chunks is free", l)
	}

	// An index chunk counts its used pages, and each holds something.
	checkIndex := func(c uint64, e chunk) {
		if e.live != e.used.count() {
			t.Fatalf("index chunk %d counts %d used pages, maps %d", c, e.live, e.used.count())
		}
		for i := range uint64(chunkPages) {
			p := c*chunkPages + i
			if _, ok := owner[p]; e.used[i/64]&(1<<(i%64)) != 0 && !ok && !pend[p] && p >= firstPage {
				t.Fatalf("index page %d used by nothing", p)
			}
		}
	}

	// A value chunk counts its live bytes, which add up to the space's, and
	// holds some or awaits release.
	var liveBytes int64
	checkValue := func(c uint64, e chunk) {
		liveBytes += int64(e.live)
		if e.live != live[c] {
			t.Fatalf("value chunk %d counts %d live bytes, holds %d", c, e.live, live[c])
		}
		if e.live == 0 && !pend[c*chunkPages] {
			t.Fatalf("value chunk %d holds nothing and awaits no release", c)
		}
	}

	// Each chunk counts what it holds and holds something, and the free and
	// partial sets list exactly the free chunks and the index chunks with
	// room.
	var free, partial []uint64
	for c, e := range sp.table {
		c := uint64(c)
		switch e.kind {
		case chunkFree:
			free = append(free, c)
		case chunkIndex:
			checkIndex(c, e)
			if e.live < chunkPages {
				partial = append(partial, c)
			}
		case chunkValue, chunkTail:
			checkValue(c, e)
		}
	}
	if liveBytes != sp.liveBytes {
		t.Fatalf("space counts %d live bytes, chunks hold %d", sp.liveBytes, liveBytes)
	}
	var gotFree, gotPartial []uint64
	sp.free.Scan(func(c uint64) bool { gotFree = append(gotFree, c); return true })
	sp.partial.Scan(func(f fullness) bool { gotPartial = append(gotPartial, f.chunk); return true })
	slices.Sort(gotPartial)
	if !slices.Equal(free, gotFree) || !slices.Equal(partial, gotPartial) {
		t.Fatalf("free %v partial %v, table has free %v partial %v", gotFree, gotPartial, free, partial)
	}

	// The space rebuilt from the file, caught up with the releases the
	// writer made without a record, equals the writer's without its
	// reservations.
	rw := &writer{db: db}
	if err := rw.replaySpace(st); err != nil {
		t.Fatal(err)
	}
	got, want := rw.sp, sp.clone()
	got.release(want.marks.seq, want.marks.ckpt)
	for _, r := range want.reserved {
		want.freeRun(r)
	}
	for c := range want.table {
		if want.table[c].kind == chunkHeld {
			want.table[c].kind = chunkFree
		}
	}
	if d := diffSpace(got, want); d != "" {
		t.Fatalf("space rebuilt from the file differs from the writer's: %s", d)
	}
}

// diffSpace describes the first difference between the saved state of a and
// b, or returns empty when they agree.
func diffSpace(a, b *space) string {
	if len(a.table) != len(b.table) {
		return fmt.Sprintf("%d chunks, want %d", len(a.table), len(b.table))
	}
	for c := range a.table {
		if a.table[c] != b.table[c] {
			return fmt.Sprintf("chunk %d is %+v, want %+v", c, a.table[c], b.table[c])
		}
	}
	switch {
	case a.open != b.open:
		return fmt.Sprintf("open %+v, want %+v", a.open, b.open)
	case !slices.Equal(a.values, b.values):
		return fmt.Sprintf("values %v, want %v", a.values, b.values)
	case !slices.Equal(a.pages, b.pages):
		return fmt.Sprintf("pages %v, want %v", a.pages, b.pages)
	case !slices.Equal(a.log, b.log):
		return fmt.Sprintf("log %v, want %v", a.log, b.log)
	case a.liveBytes != b.liveBytes:
		return fmt.Sprintf("%d live bytes, want %d", a.liveBytes, b.liveBytes)
	}
	return ""
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
// paths of its changes together instead of one path at a time.
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

	// Reopen it so its index is on disk, and let the warm-up finish before
	// the test empties the cache.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	if wait, _ := db.warmer.SetRoutine(nil); wait != nil {
		<-wait
	}

	// Delete and replace scattered keys through a cold cache whose reads
	// each take long enough to overlap with the reads beside them.
	r := &overlapReader{r: db.s}
	db.p.r = r
	db.p.cache.clear()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8000; i += 97 {
		k := fmt.Appendf(nil, "%s/%07d", prefix, i)
		if i%2 == 0 {
			err = tx.Delete(ctx, k)
		} else {
			err = tx.Set(ctx, k, []byte("new"))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if r.most.Load() < 2 {
		t.Fatalf("commit kept at most %d reads in flight", r.most.Load())
	}

	// Check the commit's result.
	rtx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	for i := 0; i < 8000; i += 97 {
		k := fmt.Appendf(nil, "%s/%07d", prefix, i)
		v, found, err := rtx.Get(ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		if found != (i%2 != 0) || (found && string(v) != "new") {
			t.Fatalf("key %d: found %v value %q", i, found, v)
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
