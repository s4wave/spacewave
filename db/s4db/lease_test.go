//go:build darwin || linux || windows

package s4db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// TestLease checks that a lease excludes other holders in the handle, that
// release wakes a waiter, and that an abandoned wait leaves the name free.
func TestLease(t *testing.T) {
	// Open the file.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "lease.s4wave"), Options{})
	defer db.Close()

	// Hold a, and check that a is busy and b is free.
	a, ok, err := db.TryLease("a")
	if err != nil || !ok {
		t.Fatalf("TryLease(a) = %v, %v", ok, err)
	}
	if _, ok, err := db.TryLease("a"); err != nil || ok {
		t.Fatalf("second TryLease(a) = %v, %v", ok, err)
	}
	b, ok, err := db.TryLease("b")
	if err != nil || !ok {
		t.Fatalf("TryLease(b) = %v, %v", ok, err)
	}
	if err := b.Release(); err != nil {
		t.Fatal(err)
	}

	// A canceled wait returns while a is held.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := db.WaitLease(canceled, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled WaitLease(a) = %v", err)
	}

	// Releasing a wakes a waiter.
	got := make(chan error, 1)
	go func() {
		l, err := db.WaitLease(ctx, "a")
		if err == nil {
			err = l.Release()
		}
		got <- err
	}()
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
	if err := <-got; err != nil {
		t.Fatal(err)
	}

	// The name is free again.
	a, ok, err = db.TryLease("a")
	if err != nil || !ok {
		t.Fatalf("TryLease(a) after release = %v, %v", ok, err)
	}
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
}
