//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

	// write sets keys of the form name to values from val, or deletes them
	// when val is nil, 64 to a commit; usage reads the file's use.
	r := rand.New(rand.NewPCG(3, 4))
	m := make(model)
	write := func(name string, keys []int, val func() []byte) {
		t.Helper()
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
	large := func() []byte {
		return bytes.Repeat([]byte{byte(r.Uint32())}, 2<<10+r.IntN(28<<10))
	}
	small := func() []byte {
		return fmt.Appendf(nil, "%064d", r.Uint64())
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
