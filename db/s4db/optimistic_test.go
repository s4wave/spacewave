package s4db

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestOptimisticCommit walks optimistic write transactions through the
// cases Commit decides: a body reading while another writer commits without
// waiting, a commit into a scanned prefix forcing a rerun, a disjoint commit
// passing validation, a deferred write reading at the commit without a
// conflict, repeated conflicts falling back to the writer lock, and a failed
// body releasing nothing it never took.
func TestOptimisticCommit(t *testing.T) {
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "optimistic.s4wave"), Options{})
	defer db.Close()
	open := func(ctx context.Context) (kvtx.Tx, error) { return db.NewTransaction(ctx, true) }
	set := func(key string, n int) {
		t.Helper()
		err := kvtx.RunTransaction(ctx, true, open, func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, []byte(key), []byte(strconv.Itoa(n)))
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	get := func(tx kvtx.Tx, key string) int {
		v, _, err := tx.Get(ctx, []byte(key))
		if err != nil {
			t.Error(err)
		}
		n, _ := strconv.Atoi(string(v))
		return n
	}
	read := func(key string) int {
		tx, err := db.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		return get(tx, key)
	}
	set("q/1", 1)
	set("q/2", 1)

	// run runs body optimistically in the background, letting each
	// attempt read and then wait for between before it commits.
	type result struct {
		attempts int
		err      error
	}
	run := func(body func(ctx context.Context, tx kvtx.Tx, attempt int) error, between func(attempt int)) result {
		reading, resume := make(chan int), make(chan struct{})
		done := make(chan result, 1)
		go func() {
			attempts := 0
			err := kvtx.RunOptimisticTransaction(ctx, open, func(ctx context.Context, tx kvtx.Tx) error {
				attempts++
				if err := body(ctx, tx, attempts); err != nil {
					return err
				}
				reading <- attempts
				<-resume
				return nil
			}, func(error) bool { return false })
			done <- result{attempts, err}
		}()
		for {
			select {
			case attempt := <-reading:
				between(attempt)
				resume <- struct{}{}
			case r := <-done:
				return r
			}
		}
	}

	// A scan of q/ waits for no writer, and a commit inserting into q/
	// while it reads forces one rerun that sees the new key.
	scan := func(ctx context.Context, tx kvtx.Tx, attempt int) error {
		sum := 0
		err := tx.ScanPrefix(ctx, []byte("q/"), func(_, v []byte) error {
			n, _ := strconv.Atoi(string(v))
			sum += n
			return nil
		})
		if err != nil {
			return err
		}
		return tx.Set(ctx, []byte("sum"), []byte(strconv.Itoa(sum)))
	}
	r := run(scan, func(attempt int) {
		if attempt == 1 {
			set("other", 1)
			set("q/3", 1)
		}
	})
	if r.err != nil || r.attempts != 2 || read("sum") != 3 {
		t.Fatalf("scan: %d attempts, sum %d, err %v; want 2 attempts, sum 3", r.attempts, read("sum"), r.err)
	}

	// A commit to a key the body did not read passes validation.
	bump := func(key string) func(context.Context, kvtx.Tx, int) error {
		return func(ctx context.Context, tx kvtx.Tx, attempt int) error {
			return tx.Set(ctx, []byte(key), []byte(strconv.Itoa(get(tx, key)+1)))
		}
	}
	r = run(bump("a"), func(int) { set("b", 1) })
	if r.err != nil || r.attempts != 1 || read("a") != 1 {
		t.Fatalf("disjoint: %d attempts, a %d, err %v; want 1 attempt, a 1", r.attempts, read("a"), r.err)
	}

	// A write deferred to the commit reads the key there, so a commit to it
	// while the body reads forces no rerun, and both writes count.
	add := func(ctx context.Context, tx kvtx.Tx, attempt int) error {
		return kvtx.AtCommit(ctx, tx, func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, []byte("n"), []byte(strconv.Itoa(get(tx, "n")+1)))
		})
	}
	r = run(add, func(int) { set("n", 5) })
	if r.err != nil || r.attempts != 1 || read("n") != 6 {
		t.Fatalf("deferred: %d attempts, n %d, err %v; want 1 attempt, n 6", r.attempts, read("n"), r.err)
	}

	// A body conflicting on every optimistic attempt runs the last one
	// holding the writer lock, and its write counts every commit before.
	locked := false
	r = run(bump("a"), func(attempt int) {
		if db.w.held.Load() {
			locked = true
			return
		}
		set("a", 10*attempt)
	})
	if r.err != nil || r.attempts != 3 || !locked || read("a") != 21 {
		t.Fatalf("fallback: %d attempts, locked %v, a %d, err %v; want 3 attempts, locked, a 21", r.attempts, locked, read("a"), r.err)
	}

	// A failed body discards an attempt that never took the lock, and
	// writers go on.
	errBody := errors.New("body failed")
	r = run(func(context.Context, kvtx.Tx, int) error { return errBody }, func(int) {})
	if !errors.Is(r.err, errBody) || r.attempts != 1 {
		t.Fatalf("failed body: %d attempts, err %v", r.attempts, r.err)
	}
	set("c", 1)
	if read("c") != 1 {
		t.Fatal("a writer after a failed optimistic body lost its write")
	}
}
