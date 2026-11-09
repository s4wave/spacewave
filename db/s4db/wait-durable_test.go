package s4db

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestWaitDurableAfterBackgroundRecord makes an ordered commit durable, then
// appends a background record that arms no flush, as a release or cleaner
// step does on a quiet file. WaitDurable waits for the ordered commits only,
// so it returns instead of waiting for a commit that may never come.
func TestWaitDurableAfterBackgroundRecord(t *testing.T) {
	// Open a file and make one ordered commit durable.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "wait.s4wave"), Options{CacheBytes: 4 << 20})
	defer db.Close()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.WaitDurable(wctx); err != nil {
		t.Fatalf("ordered commit: %v", err)
	}

	// Publish a background record past the durable mark.
	durable, _ := db.flush.marks()
	err = db.w.locked(ctx, func() error {
		_, err := db.w.appendRecord(ctx, &record{kind: kindCommit})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if seq := db.cur.Load().seq; seq <= durable {
		t.Fatalf("background record seq %d is not past the durable mark %d", seq, durable)
	}

	// The wait ends well before any later commit would arm a flush.
	wctx, cancel = context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := db.WaitDurable(wctx); err != nil {
		t.Fatalf("wait after a background record: %v", err)
	}
}
