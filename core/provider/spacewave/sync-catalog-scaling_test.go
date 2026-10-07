//go:build !js && !wasip1

package provider_spacewave

import (
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/s4db"
)

// TestSyncCatalogChangeScaling measures one committed pack acknowledgement.
func TestSyncCatalogChangeScaling(t *testing.T) {
	for _, count := range []int{1000, 10000, 100000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			// Open the native volume backend and release it after the read store.
			ctx := t.Context()
			backend, err := s4db.Open(filepath.Join(t.TempDir(), "catalog.s4wave"), s4db.Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := backend.Close(); err != nil {
					t.Error(err)
				}
			})

			// Retain a catalog with startup publication excluded from measurement.
			catalog, err := manifest.New(ctx, backend)
			if err != nil {
				t.Fatal(err)
			}
			syncer := &syncController{mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil)}
			t.Cleanup(syncer.lower.Close)
			entries := make([]*packfile.PackfileEntry, count)
			for n := range count {
				entries[n] = &packfile.PackfileEntry{Id: "pack-" + strconv.Itoa(n), Sequence: uint64(n + 1), BlockCount: 1, SizeBytes: 100}
			}
			if err := syncer.applyManifestDelta(ctx, entries, nil, uint64(count)); err != nil {
				t.Fatal(err)
			}
			beforeEntries := syncer.lower.SnapshotManifest().GetEntries()

			// Measure durable mutation and provider publication of one local pack.
			added := &packfile.PackfileEntry{Id: "new-pack", BlockCount: 1, SizeBytes: 100}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			started := time.Now()
			if err := syncer.applyManifestDelta(ctx, []*packfile.PackfileEntry{added}, nil, 0); err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started)
			runtime.ReadMemStats(&after)

			// Count replaced read descriptors outside the timed operation.
			previous := make(map[string]*packfile.PackfileEntry, count)
			for _, entry := range beforeEntries {
				previous[entry.GetId()] = entry
			}
			touched := 0
			for _, entry := range syncer.lower.SnapshotManifest().GetEntries() {
				if previous[entry.GetId()] != entry {
					touched++
				}
			}
			if touched != 1 {
				t.Fatalf("single acknowledgement rebuilt %d catalog descriptors", touched)
			}
			t.Logf("backend=s4db retained=%d changed=1 descriptors=%d allocated=%d mallocs=%d elapsed=%s", count, touched, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, elapsed)
		})
	}
}
