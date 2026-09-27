//go:build !js && !wasip1

package provider_spacewave

import (
	"context"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
)

// TestSyncCatalogChangeScaling measures durable mutation and reader publication together.
func TestSyncCatalogChangeScaling(t *testing.T) {
	for _, count := range []int{1000, 10000, 100000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			ctx := t.Context()
			backend, err := store_kvtx_bolt.Open(filepath.Join(t.TempDir(), "catalog.db"), 0600, nil, []byte("metadata"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { backend.Close() })
			if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
				return backend.NewTransaction(ctx, true)
			}, func(ctx context.Context, tx kvtx.Tx) error {
				return tx.Set(ctx, []byte("fixture"), []byte("catalog"))
			}); err != nil {
				t.Fatal(err)
			}
			catalog, err := manifest.New(ctx, backend)
			if err != nil {
				t.Fatal(err)
			}
			syncer := &syncController{mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil)}
			entries := make([]*packfile.PackfileEntry, count)
			for n := range entries {
				entries[n] = &packfile.PackfileEntry{Id: "pack-" + strconv.Itoa(n), Sequence: uint64(n + 1), BlockCount: 1, SizeBytes: 100}
			}
			if err := syncer.applyManifestDelta(ctx, entries, nil); err != nil {
				t.Fatal(err)
			}
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			started := time.Now()
			if err := syncer.applyManifestDelta(ctx, []*packfile.PackfileEntry{{Id: "new-pack", BlockCount: 1, SizeBytes: 100}}, nil); err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started)
			runtime.ReadMemStats(&after)
			t.Logf("retained=%d allocated=%d elapsed=%s", count, after.TotalAlloc-before.TotalAlloc, elapsed)
		})
	}
}
