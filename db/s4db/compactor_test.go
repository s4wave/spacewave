//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

// fillSlack writes two generations of keys, deletes the first and releases
// its space, so the second lies past the goal with free space below it. It
// returns the keys left.
func fillSlack(t *testing.T, ctx context.Context, db *DB) model {
	// Write or delete one generation of 512 keys in batches of 16.
	t.Helper()
	m := make(model)
	put := func(gen int, del bool) {
		for i := 0; i < 512; i += 16 {
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			for j := i; j < i+16; j++ {
				key := fmt.Sprintf("g%d/%04d", gen, j)
				if del {
					err = tx.Delete(ctx, []byte(key))
					delete(m, key)
				} else {
					val := bytes.Repeat([]byte{byte(gen), byte(j)}, 2500)
					err = tx.Set(ctx, []byte(key), val)
					m[key] = val
				}
				if err != nil {
					tx.Discard()
					t.Fatal(err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	put(0, false)
	put(1, false)
	put(0, true)

	// Checkpoint twice, so the deleted values' pages are released.
	w := db.w
	for range 2 {
		if err := w.lock(ctx); err != nil {
			t.Fatal(err)
		}
		err := w.checkpointDurable(ctx)
		if err == nil {
			err = w.releaseNow(ctx)
		}
		w.unlock(err)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Wait until the background punch returns its reserved runs to the free
	// space that the compactor plans against.
	_ = w.punches.WaitIdle(context.Background(), nil)
	return m
}

// TestCompactorClose closes handles while the background compactor works
// through a file with slack, and checks that each reopen holds every key.
func TestCompactorClose(t *testing.T) {
	// Fill a file with slack, with the background compactor.
	ctx := context.Background()
	opts := Options{SingleProcess: true}
	path := filepath.Join(t.TempDir(), "close.s4wave")
	db := openTest(t, path, opts)
	m := fillSlack(t, ctx, db)

	// Close and reopen as the compactor works, checking every key.
	for range 8 {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openTest(t, path, opts)
		m.check(t, db)
		checkSpace(t, db)
	}

	// Close the last handle.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestCompactorScanCurrentValue checks that a scan lists the value a key
// holds now, not the one in the checkpoint's index, and skips a key deleted
// since: the index's values may lie in space a commit freed and reused.
func TestCompactorScanCurrentValue(t *testing.T) {
	// Fill a file with slack, without the background compactor.
	ctx := context.Background()
	opts := Options{RelocateBudget: -1, SingleProcess: true}
	db := openTest(t, filepath.Join(t.TempDir(), "current.s4wave"), opts)
	defer db.Close()
	fillSlack(t, ctx, db)

	// Change one key and delete another after the checkpoint.
	changed, deleted := []byte("g1/0005"), []byte("g1/0006")
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, changed, bytes.Repeat([]byte{9}, 6000)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Delete(ctx, deleted); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Scan every key on a snapshot whose overlay holds both changes.
	c := &compactor{db: db}
	if ok, _, err := c.plan(ctx); err != nil || !ok {
		t.Fatalf("plan: ok=%v err=%v", ok, err)
	}
	st, stripe := db.acquire()
	defer db.release(st, stripe)
	if !st.filter.has(changed) || !st.filter.has(deleted) {
		t.Fatal("a checkpoint took the changes into the index")
	}
	refs, _, err := c.scan(ctx, st)
	if err != nil {
		t.Fatal(err)
	}

	// The changed key lists its current value and the deleted key none.
	want, _, err := db.lookup(st, changed)
	if err != nil {
		t.Fatal(err)
	}
	var sawChanged bool
	for _, m := range refs {
		switch string(m.key) {
		case string(deleted):
			t.Fatal("scan listed a deleted key")
		case string(changed):
			sawChanged = true
			if !sameValue(m.val, want) {
				t.Fatalf("scan listed %+v, want the current value %+v", m.val.ref, want.ref)
			}
		}
	}
	if !sawChanged {
		t.Fatal("scan did not list the changed key")
	}
}

// TestCompactorRestsAfterLittle checks that the rest after a pass that moved
// and took off the end less than the least slack worth compacting doubles up
// to compactRestMax, and that a pass that moved or lowered the end enough
// continues at once and starts the doubling over.
func TestCompactorRestsAfterLittle(t *testing.T) {
	// Passes that move a little and lower the end a little rest longer each
	// time, up to the bound.
	c := &compactor{end: 1 << 20}
	want := compactRest
	for range 8 {
		c.passFound, c.passMoved = 1, pageOff(compactMinSlack)-1
		c.end -= compactMinSlack - 1
		if got := c.endPass(); got != want {
			t.Fatalf("rest %v, want %v", got, want)
		}
		if !c.flush {
			t.Fatal("a pass that found values asked for no checkpoint")
		}
		want = min(2*want, compactRestMax)
	}

	// A pass that moves enough does not rest, and the next little one
	// rests the least.
	c.passMoved = pageOff(compactMinSlack)
	if got := c.endPass(); got != 0 {
		t.Fatalf("rest %v after a full pass, want none", got)
	}
	if got := c.endPass(); got != compactRest || c.flush {
		t.Fatalf("rest %v flush %v after an empty pass, want %v and no checkpoint", got, c.flush, compactRest)
	}

	// A pass after the end fell enough does not rest either.
	c.end -= compactMinSlack
	if got := c.endPass(); got != 0 {
		t.Fatalf("rest %v after the end fell %d pages, want none", got, compactMinSlack)
	}
}

// TestCompactorEmptiesSparsePages checks that the compactor moves values out
// of sparse pages that no free run below would take, as commits no longer do.
func TestCompactorEmptiesSparsePages(t *testing.T) {
	// Fill pages with four values each, without the background compactor.
	ctx := context.Background()
	opts := Options{RelocateBudget: -1, SingleProcess: true}
	db := openTest(t, filepath.Join(t.TempDir(), "sparse.s4wave"), opts)
	defer db.Close()
	val := bytes.Repeat([]byte{1}, 1000)
	commit := func(fn func(tx kvtx.Tx) error) {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := fn(tx); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	commit(func(tx kvtx.Tx) error {
		for i := range 4000 {
			if err := tx.Set(ctx, fmt.Appendf(nil, "k%05d", i), val); err != nil {
				return err
			}
		}
		return nil
	})

	// Delete three values of four, leaving every page sparse.
	commit(func(tx kvtx.Tx) error {
		for i := range 4000 {
			if i%4 == 0 {
				continue
			}
			if err := tx.Delete(ctx, fmt.Appendf(nil, "k%05d", i)); err != nil {
				return err
			}
		}
		return nil
	})

	// Checkpoint, so the index the compactor scans holds the keys.
	w := db.w
	if err := w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	err := w.checkpointDurable(ctx)
	w.unlock(err)
	if err != nil {
		t.Fatal(err)
	}

	// Step the compactor by hand: the pages, a quarter live, empty until
	// what is free inside value pages falls below the least worth moving.
	// Each step waits for the background punch, which holds the runs it
	// punches out of the free runs the moves fill.
	c := &compactor{db: db}
	for range 20 {
		_ = db.w.punches.WaitIdle(context.Background(), nil)
		if _, err := c.step(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if live, held := db.w.sp.stats(); held > 2*live {
		t.Fatalf("%d live bytes in %d held after compaction, want at most twice", live, held)
	}
}
