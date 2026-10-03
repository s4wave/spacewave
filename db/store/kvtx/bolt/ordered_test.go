//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// openOrderedTestStore opens a store in a temporary directory.
func openOrderedTestStore(t *testing.T) *Store {
	// Open a temporary Bolt store and register its close cleanup.
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "db"), 0o600, nil, []byte("ordered"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// commitTestKey writes key, committing ordered or in full.
func commitTestKey(t *testing.T, s *Store, key string, ordered bool) {
	// Write the test key using the requested commit durability mode.
	t.Helper()
	tx, err := s.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(t.Context(), []byte(key), []byte(key)); err != nil {
		t.Fatal(err)
	}
	if ordered {
		err = kvtx.CommitOrdered(t.Context(), tx)
	} else {
		err = tx.Commit(t.Context())
	}
	if err != nil {
		t.Fatal(err)
	}
}

// checkPending fails unless an ordered commit is pending exactly when want.
func checkPending(t *testing.T, s *Store, want bool) {
	t.Helper()
	if got := s.durable.Load() < s.ordered.Load(); got != want {
		t.Fatalf("pending = %v, want %v", got, want)
	}
}

// TestStoreSyncCoversOrderedCommits proves Sync flushes only while an ordered
// commit is pending, and a full commit covers the ordered commits before it.
func TestStoreSyncCoversOrderedCommits(t *testing.T) {
	// Open the Bolt store for explicit sync coverage checks.
	s := openOrderedTestStore(t)

	// Verify explicit Sync makes one ordered commit durable.
	commitTestKey(t, s, "a", true)
	checkPending(t, s, true)
	if err := s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkPending(t, s, false)

	// Verify a full commit also makes earlier ordered commits durable.
	commitTestKey(t, s, "b", true)
	commitTestKey(t, s, "c", true)
	checkPending(t, s, true)
	commitTestKey(t, s, "d", false)
	checkPending(t, s, false)
}

// TestStoreDeadlineFlushesOrderedCommits proves WaitDurable returns once the
// deadline flushes a pending ordered commit, with no caller forcing a flush.
func TestStoreDeadlineFlushesOrderedCommits(t *testing.T) {
	// Verify a store with no pending ordered commits is already durable.
	s := openOrderedTestStore(t)
	if err := s.WaitDurable(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Create pending ordered commits for the deadline flush.
	commitTestKey(t, s, "a", true)
	commitTestKey(t, s, "b", true)
	checkPending(t, s, true)

	// Wait for the deadline flush within a bounded context.
	ctx, cancel := context.WithTimeout(t.Context(), 5*SyncDeadline)
	defer cancel()
	start := time.Now()
	if err := s.WaitDurable(ctx); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*SyncDeadline {
		t.Fatalf("deadline flush took %v", elapsed)
	}
	checkPending(t, s, false)
}

// TestStoreCloseFlushesOrderedCommits proves Close makes pending ordered
// commits durable before closing the database.
func TestStoreCloseFlushesOrderedCommits(t *testing.T) {
	// Verify closing the Store makes pending ordered commits durable.
	s := openOrderedTestStore(t)
	commitTestKey(t, s, "a", true)
	checkPending(t, s, true)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	checkPending(t, s, false)
}
