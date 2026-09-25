//go:build !js && !wasip1

package kvtx

import (
	"context"
	"path/filepath"
	"testing"

	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
)

// orderedFlushStore counts the flushes Volume.Sync requests.
type orderedFlushStore struct {
	*store_bolt.Store
	flushes int
}

func (s *orderedFlushStore) Sync(ctx context.Context) error {
	s.flushes++
	return s.Store.Sync(ctx)
}

// TestVolumeSyncFlushesOrderedCommits proves a direct preparation commits
// with write ordering and Sync flushes it exactly once.
func TestVolumeSyncFlushesOrderedCommits(t *testing.T) {
	raw, err := store_bolt.Open(filepath.Join(t.TempDir(), "db"), 0o600, nil, []byte("ordered"))
	if err != nil {
		t.Fatal(err)
	}
	s := &orderedFlushStore{Store: raw}
	v, err := NewVolume(t.Context(), "ordered", store_kvkey.NewDefaultKVKey(), s, &store_kvtx.Config{}, false, false, nil, raw.GetDB().Close)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	})
	if v.ordered == nil {
		t.Fatal("bolt volume does not use ordered commits")
	}

	sync := func(want int) {
		t.Helper()
		if _, err := v.Sync(t.Context()); err != nil {
			t.Fatal(err)
		}
		if s.flushes != want {
			t.Fatalf("flushes = %d, want %d", s.flushes, want)
		}
	}
	sync(0)
	ref, _, err := v.PrepareOwnedBlock(t.Context(), "bucket", []byte("ordered block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	sync(1)
	sync(1)

	data, found, err := v.GetBlock(t.Context(), ref)
	if err != nil || !found || string(data) != "ordered block" {
		t.Fatalf("GetBlock = %q, %v, %v", data, found, err)
	}
}
