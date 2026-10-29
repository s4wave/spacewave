//go:build darwin || linux || windows

package s4db

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

// TestScanHidesOverlayDeletes checks that prefix scans, seeks and reverse
// scans hide the tree keys the overlay deletes, and show a buffered write
// over such a delete.
func TestScanHidesOverlayDeletes(t *testing.T) {
	// Open a file without the background cleaner.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "dead.s4wave"), Options{SingleProcess: true})
	defer db.Close()
	if wait, _ := db.cleaner.SetRoutine(nil); wait != nil {
		<-wait
	}

	// Commit ten keys under a prefix and fold them into the tree.
	key := func(i int) []byte { return fmt.Appendf(nil, "p/%02d", i) }
	commit := func(fn func(tx *Tx)) {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		fn(tx.(*Tx))
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	commit(func(tx *Tx) {
		_ = tx.Set(ctx, []byte("o/x"), []byte("v"))
		for i := range 10 {
			_ = tx.Set(ctx, key(i), []byte("v"))
		}
	})
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	err := db.w.checkpointDurable(ctx)
	db.w.unlock(err)
	if err != nil {
		t.Fatal(err)
	}

	// Delete the even keys in the overlay, with one the tree lacks.
	commit(func(tx *Tx) { _ = tx.Set(ctx, key(10), []byte("v")) })
	commit(func(tx *Tx) {
		for i := 0; i <= 10; i += 2 {
			_ = tx.Delete(ctx, key(i))
		}
	})
	if st := db.cur.Load(); st.overlay.dead.Len() != 6 || st.overlay.live.Len() != 0 {
		t.Fatalf("overlay holds %d deletes and %d live entries, want 6 and 0", st.overlay.dead.Len(), st.overlay.live.Len())
	}

	// A read sees the odd keys forward, from a seek, and backward.
	odd := []string{"p/01", "p/03", "p/05", "p/07", "p/09"}
	rtx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rtx.Discard()
	checkScans(t, rtx.(*Tx), odd)

	// A write sees its buffered write over a delete.
	wtx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer wtx.Discard()
	if err := wtx.Set(ctx, key(4), []byte("v")); err != nil {
		t.Fatal(err)
	}
	checkScans(t, wtx.(*Tx), []string{"p/01", "p/03", "p/04", "p/05", "p/07", "p/09"})
}

// checkScans checks the keys under p/ forward, from a seek to p/04, and
// backward against want.
func checkScans(t *testing.T, tx *Tx, want []string) {
	// List the keys each scan should see.
	t.Helper()
	fromSeek := slices.DeleteFunc(slices.Clone(want), func(k string) bool { return k < "p/04" })
	backward := slices.Clone(want)
	slices.Reverse(backward)

	// Run each scan.
	for _, c := range []struct {
		seek    []byte
		reverse bool
		want    []string
	}{
		{nil, false, want},
		{[]byte("p/04"), false, fromSeek},
		{nil, true, backward},
	} {
		if got := scanKeys(t, tx, c.seek, c.reverse); !slices.Equal(got, c.want) {
			t.Fatalf("scan from %q reverse %v: got %v, want %v", c.seek, c.reverse, got, c.want)
		}
	}
}

// scanKeys returns the keys under p/ from seek, or from the start.
func scanKeys(t *testing.T, tx *Tx, seek []byte, reverse bool) []string {
	// Position the iterator.
	it := tx.Iterate(context.Background(), []byte("p/"), true, reverse)
	defer it.Close()
	if seek != nil {
		if err := it.Seek(seek); err != nil {
			t.Fatal(err)
		}
	} else {
		it.Next()
	}

	// Collect the keys.
	var got []string
	for ; it.Valid(); it.Next() {
		got = append(got, string(it.Key()))
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

// BenchmarkScanAfterChurn moves one owner's single edge n times, leaving n
// deletes under its prefix in the overlay, then scans the prefix.
func BenchmarkScanAfterChurn(b *testing.B) {
	for _, n := range []int{0, 1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			// Move the edge n times.
			ctx := context.Background()
			db, err := Open(filepath.Join(b.TempDir(), "churn.s4wave"), Options{})
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			prefix := []byte("f/head:owner/")
			edge := func(i int) []byte { return fmt.Appendf(nil, "%sblock%08d", prefix, i) }
			for i := range n + 1 {
				tx, err := db.NewTransaction(ctx, true)
				if err != nil {
					b.Fatal(err)
				}
				_ = tx.Set(ctx, edge(i), []byte{})
				if i > 0 {
					_ = tx.Delete(ctx, edge(i-1))
				}
				if err := tx.Commit(ctx); err != nil {
					b.Fatal(err)
				}
			}

			// Scan the prefix, which holds one live edge.
			for b.Loop() {
				tx, err := db.NewTransaction(ctx, false)
				if err != nil {
					b.Fatal(err)
				}
				var n int
				err = tx.ScanPrefixKeys(ctx, prefix, func([]byte) error { n++; return nil })
				tx.Discard()
				if err != nil || n != 1 {
					b.Fatalf("scan found %d keys: %v", n, err)
				}
			}
		})
	}
}
