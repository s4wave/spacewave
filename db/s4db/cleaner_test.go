//go:build darwin || linux || windows

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
	"time"

	"github.com/s4wave/spacewave/db/volume/device"
)

// TestCleaner walks the cleaner through a file that fills, loses values in
// random order, and keeps taking writes. Compact empties the sparse value
// chunks, so the value chunks end within an eighth of the live bytes and the
// file shrinks; the moves count in no flow; the space matches the file and
// the values read back after reopening; the background cleaner shrinks the
// value chunks again by itself; and Compact gathers a thinned index spread
// over many chunks into a few.
func TestCleaner(t *testing.T) {
	// Open without the background cleaner.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "clean.s4wave")
	opts := Options{RelocateBudget: -1, CheckpointMin: 256 << 10, CheckpointMax: 1 << 20, Families: [][]byte{[]byte("k/")}}
	db := openTest(t, path, opts)

	// write sets or deletes keys, and usage reads the file's use.
	r := rand.New(rand.NewPCG(3, 4))
	m := make(model)
	large, small := largeValues(r), smallValues(r, 64)
	write := func(name string, keys []int, val func() []byte) {
		t.Helper()
		setKeys(t, db, m, name, keys, val)
	}
	usage := func() Usage {
		t.Helper()
		u, err := db.Usage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	// tight reports whether the value chunks hold at most an eighth more
	// than the live values, plus the two streams' open chunks.
	tight := func(u Usage) bool {
		return u.ValueChunkBytes <= u.ValueBytes+u.ValueBytes/8+2*chunkSize
	}

	// Fill 2,000 keys, delete 40% in another order, and rewrite a fifth.
	write("k/%05d", r.Perm(2000), large)
	keys := r.Perm(2000)
	write("k/%05d", keys[:800], nil)
	write("k/%05d", keys[800:1200], large)
	before := usage()
	if tight(before) {
		t.Fatalf("value chunks already tight before cleaning: %+v", before)
	}

	// Compact cleans: the value chunks tighten, the file shrinks to them,
	// the index and the log, and the flows do not change.
	flows := db.Flows()
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	after := usage()
	t.Logf("before %+v, after %+v", before, after)
	if !tight(after) || after.FileBytes > after.ValueChunkBytes+after.IndexChunkBytes+after.LogBytes+2*chunkSize {
		t.Fatalf("compact left value chunks %d for %d live, file %d from %d", after.ValueChunkBytes, after.ValueBytes, after.FileBytes, before.FileBytes)
	}
	if got := db.Flows(); !slices.Equal(got, flows) {
		t.Fatalf("flows after cleaning %v, want %v", got, flows)
	}
	checkSpace(t, db)
	m.check(t, db)

	// Reopened with the background cleaner, the values read back.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	least := cleanMinSlack
	cleanMinSlack = 0
	defer func() { cleanMinSlack = least }()
	opts.RelocateBudget = 64 << 20
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)

	// After another round of deletes, with rewrites going on, the cleaner
	// tightens the value chunks alone.
	write("k/%05d", keys[1200:1700], nil)
	rewrite := keys[1700:]
	for deadline := time.Now().Add(20 * time.Second); len(rewrite) != 0 || !tight(usage()); time.Sleep(20 * time.Millisecond) {
		// Keep writing while the cleaner runs.
		if time.Now().After(deadline) {
			t.Fatalf("background cleaner left %+v", usage())
		}
		n := min(32, len(rewrite))
		write("k/%05d", rewrite[:n], large)
		rewrite = rewrite[n:]
	}
	checkSpace(t, db)
	m.check(t, db)

	// An index spread over many chunks and then thinned leaves its pages
	// scattered; Compact gathers them into a few chunks and the file
	// shrinks.
	write("k/i%06d", r.Perm(40000), small)
	write("k/i%06d", r.Perm(40000)[:34000], nil)
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	idx := usage()
	t.Logf("index %+v", idx)
	if idx.IndexChunkBytes > idx.IndexBytes+idx.IndexBytes/8+2*chunkSize {
		t.Fatalf("compact left index chunks %d for %d of pages", idx.IndexChunkBytes, idx.IndexBytes)
	}
	checkSpace(t, db)
	m.check(t, db)
}

// setKeys sets the keys of the form name numbered keys in db and m to values
// from val, or deletes them when val is nil, 64 to a durable commit.
func setKeys(t *testing.T, db *DB, m model, name string, keys []int, val func() []byte) {
	t.Helper()
	ctx := context.Background()
	for len(keys) != 0 {
		// Set or delete the next keys.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range keys[:min(64, len(keys))] {
			key := fmt.Sprintf(name, k)
			if val == nil {
				err = tx.Delete(ctx, []byte(key))
				delete(m, key)
			} else {
				v := val()
				err = tx.Set(ctx, []byte(key), v)
				m[key] = v
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		keys = keys[min(64, len(keys)):]
	}
}

// largeValues returns values of 2 to 30 KiB, stored in value chunks.
func largeValues(r *rand.Rand) func() []byte {
	return func() []byte {
		return bytes.Repeat([]byte{byte(r.Uint32())}, 2<<10+r.IntN(28<<10))
	}
}

// smallValues returns values of n bytes, at most InlineMax, stored in the
// index.
func smallValues(r *rand.Rand, n int) func() []byte {
	return func() []byte {
		return bytes.Repeat([]byte{byte(r.Uint32())}, n)
	}
}

// TestCleanerCrash crashes the device at each call of a Compact that empties
// value and index chunks, loses power, and checks that recovery keeps every
// value, which Compact does not change, and a consistent space that later
// commits and a reopen keep.
func TestCleanerCrash(t *testing.T) {
	ctx := context.Background()
	opts := Options{RelocateBudget: -1, CheckpointMin: 64 << 10, CheckpointMax: 256 << 10}
	for k := 0; ; k++ {
		// Fill a file with value and index chunks to empty, every commit
		// durable.
		r := rand.New(rand.NewPCG(5, 6))
		mem := device.NewMemory()
		db := openDevice(t, mem, opts)
		m := make(model)
		setKeys(t, db, m, "k/%05d", r.Perm(400), largeValues(r))
		setKeys(t, db, m, "k/i%05d", r.Perm(12000), smallValues(r, 400))
		setKeys(t, db, m, "k/%05d", r.Perm(400)[:240], nil)
		setKeys(t, db, m, "k/i%05d", r.Perm(12000)[:10000], nil)
		before, err := db.Usage(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Crash at Compact's call k, until Compact makes fewer.
		calls := mem.Calls()
		mem.CrashAfter(k)
		err = db.Compact(ctx)
		if err == nil {
			// Uncrashed, Compact leaves the file tight.
			after, err := db.Usage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("compact finished at k %d in %d calls: %+v to %+v", k, mem.Calls()-calls, before, after)
			if after.FreeBytes != 0 || after.FileBytes >= before.FileBytes || after.ValueChunkBytes > after.ValueBytes+after.ValueBytes/8+2*chunkSize {
				t.Fatalf("compact left %+v from %+v", after, before)
			}
		}
		_ = db.Close()
		mem.PowerLoss(r)

		// Recovery keeps every value, and later commits use its space.
		db = openDevice(t, mem, opts)
		if got := contents(t, db); !maps.EqualFunc(got, m, bytes.Equal) {
			t.Fatalf("crash at call %d: recovered %d keys, want %d", k, len(got), len(m))
		}
		checkSpace(t, db)
		for range 20 {
			randomCommit(t, r, db, m, 2000)
		}
		checkSpace(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDevice(t, mem, opts)
		m.check(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if err == nil {
			return
		}
	}
}
