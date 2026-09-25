//go:build !js && !wasip1

package store_kvtx_bolt

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestStoreSyncCoversOrderedCommits proves Sync flushes only while an ordered
// commit is pending, and a full commit covers the ordered commits before it.
func TestStoreSyncCoversOrderedCommits(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "db"), 0o600, nil, []byte("ordered"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.GetDB().Close() })

	commit := func(key string, ordered bool) {
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
	pending := func(want bool) {
		t.Helper()
		if got := s.durable.Load() < s.ordered.Load(); got != want {
			t.Fatalf("pending = %v, want %v", got, want)
		}
	}

	commit("a", true)
	pending(true)
	if err := s.Sync(t.Context()); err != nil {
		t.Fatal(err)
	}
	pending(false)

	commit("b", true)
	commit("c", true)
	pending(true)
	commit("d", false)
	pending(false)
}
