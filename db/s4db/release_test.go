package s4db

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
)

// TestReleaseAfterLastDelete deletes values in the last commit and checks
// that Release returns their runs, which no later commit releases, to the
// file system.
func TestReleaseAfterLastDelete(t *testing.T) {
	// Open without the background cleaner, which releases on its own.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "release.s4wave"), Options{})
	defer db.Close()
	if wait, _ := db.cleaner.SetRoutine(nil); wait != nil {
		<-wait
	}

	// Write 16 values of 1 MiB, then delete them in the last commit.
	keys := make([][]byte, 16)
	for i := range keys {
		keys[i] = []byte("value-" + strconv.Itoa(i))
	}
	commitKeys(t, db, keys, make([]byte, 1<<20))
	commitKeys(t, db, keys, nil)

	// The deleted runs wait for a commit that will not come.
	before, err := db.SpaceReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.PendingValueRuns == 0 {
		t.Fatal("no deleted value run is pending")
	}

	// Release returns them, and the file system no longer holds them.
	if err := db.Release(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := db.SpaceReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.PendingValueRuns != 0 {
		t.Fatalf("%d value runs pending after Release", after.PendingValueRuns)
	}
	if before.DiskBytes >= 0 && before.DiskBytes-after.DiskBytes < 12<<20 {
		t.Fatalf("disk bytes went from %d to %d", before.DiskBytes, after.DiskBytes)
	}
}

// commitKeys sets each key to val in one commit, or deletes them for a nil
// val.
func commitKeys(t *testing.T, db *DB, keys [][]byte, val []byte) {
	// Write each key in one transaction.
	t.Helper()
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, k := range keys {
		if val == nil {
			err = tx.Delete(ctx, k)
		} else {
			err = tx.Set(ctx, k, val)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// Commit it.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
