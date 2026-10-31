package s4db

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"
)

// waitFlushed waits until the memtable is back within its budget and at
// least one run exists, or fails after a while.
func waitFlushed(t *testing.T, db *DB) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for st := db.cur.Load(); st.mem.bytes > db.budget || len(st.m.runs) == 0; st = db.cur.Load() {
		if time.Now().After(deadline) {
			t.Fatalf("memtable still holds %d bytes, %d runs", st.mem.bytes, len(st.m.runs))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestFlush commits past a small memtable budget so the scheduler flushes
// runs, then reads every key through the runs, scans across them, and
// reopens the file so the superblock's manifest and the log after it rebuild
// the same contents.
func TestFlush(t *testing.T) {
	// Open a file with a small budget.
	path := filepath.Join(t.TempDir(), "flush.s4wave")
	opts := Options{CacheBytes: 4 << 20, memtable: 128 << 10}
	db := openTest(t, path, opts)
	r := rand.New(rand.NewPCG(3, 4))
	m := make(model)

	// Fill past the budget several times, checking as it goes.
	for range 12 {
		for range 10 {
			randomCommit(t, r, db, m, 5000)
		}
		m.check(t, db)
	}
	waitFlushed(t, db)
	m.check(t, db)
	if runs := len(db.cur.Load().m.runs); runs == 0 {
		t.Fatal("no run was flushed")
	}

	// Reopen: the manifest and the log after it hold the same contents.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)
	if got := contents(t, db); !maps.EqualFunc(got, m, bytes.Equal) {
		t.Fatalf("reopened contents differ: %d keys, want %d", len(got), len(m))
	}

	// Once the background jobs settle, the file holds runs and nothing stays
	// reserved.
	s := waitSettled(t, db)
	if s.Chunks.Run == 0 || s.Chunks.Reserved != 0 {
		t.Fatalf("chunks %+v", s.Chunks)
	}
}

// TestRunBuilderRoundTrip writes keys through the builder into a file and
// reads them back through a reader: present keys, absent keys, and a scan.
func TestRunBuilderRoundTrip(t *testing.T) {
	// Open a fresh file and a builder over four chunks of it.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "run.s4wave"), Options{})
	defer db.Close()
	chunks := []uint64{10, 11, 12, 13}
	rb := newRunBuilder(db.s, chunks, func() (uint64, error) { return 0, fmt.Errorf("out of chunks") })

	// Build a run of many small keys.
	const n = 20000
	for i := range n {
		k := fmt.Appendf(nil, "key/%08d", i*2)
		if err := rb.add(k, value{kind: valueInline, inline: fmt.Appendf(nil, "v%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := rb.finish()
	if err != nil {
		t.Fatal(err)
	}
	meta.id = 16
	if meta.count != n || len(meta.chunks) == 0 {
		t.Fatalf("meta %+v", meta)
	}

	// Read even keys back and miss odd ones.
	rr, err := db.reader(meta)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2*n; i += 37 {
		k := fmt.Appendf(nil, "key/%08d", i)
		v, ok, err := rr.get(k)
		if err != nil {
			t.Fatal(err)
		}
		if ok != (i%2 == 0) || (ok && string(v.inline) != fmt.Sprintf("v%d", i/2)) {
			t.Fatalf("key %d: found %v value %q", i, ok, v.inline)
		}
	}

	// Scan forward and backward through the run layer.
	it := &runIter{db: db, meta: meta}
	it.seek(nil, false)
	count := 0
	for it.valid() {
		if string(it.key()) != fmt.Sprintf("key/%08d", count*2) {
			t.Fatalf("entry %d is %q", count, it.key())
		}
		count++
		it.step(false)
	}
	if it.err != nil || count != n {
		t.Fatalf("forward scan: %d entries, %v", count, it.err)
	}
	it.seek(nil, true)
	for i := n - 1; i >= 0; i-- {
		if !it.valid() || string(it.key()) != fmt.Sprintf("key/%08d", i*2) {
			t.Fatalf("reverse entry %d is %q (%v)", i, it.key(), it.err)
		}
		it.step(true)
	}
	_ = ctx
}
