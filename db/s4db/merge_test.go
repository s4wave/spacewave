package s4db

import (
	"bytes"
	"context"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"
)

// waitMerged waits until level 0 holds at most level0Trigger runs and a lower
// level exists, or fails after a while.
func waitMerged(t *testing.T, db *DB) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		st := db.cur.Load()
		if st.m.level0() <= level0Trigger && len(st.m.runs) > st.m.level0() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d runs, %d at level 0", len(st.m.runs), st.m.level0())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestMerge fills past the flush budget many times so level 0 passes its
// trigger and the merger combines runs into level 1, then checks every key
// and scan against the model, reopens, and checks that the merged inputs'
// chunks were freed.
func TestMerge(t *testing.T) {
	// Open a file with a small budget and fill it through many flushes.
	path := filepath.Join(t.TempDir(), "merge.s4wave")
	opts := Options{CacheBytes: 4 << 20, memtable: 64 << 10}
	db := openTest(t, path, opts)
	r := rand.New(rand.NewPCG(5, 6))
	m := make(model)
	for range 40 {
		for range 6 {
			randomCommit(t, r, db, m, 8000)
		}
	}
	waitMerged(t, db)
	m.check(t, db)

	// Reopen: the merged manifest and the log after it hold the contents.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)
	if got := contents(t, db); !maps.EqualFunc(got, m, bytes.Equal) {
		t.Fatalf("reopened contents differ: %d keys, want %d", len(got), len(m))
	}

	// The inputs' chunks were released: nothing stays reserved, and pending
	// chunks are few.
	s, err := db.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Chunks.Reserved != 0 || len(s.Runs) < 2 {
		t.Fatalf("chunks %+v runs %v", s.Chunks, s.Runs)
	}
}
