//go:build !js && !wasip1

package store

import (
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/packfile"
)

// TestManifestDeltaReaderScaling measures publication with every retained reader open.
func TestManifestDeltaReaderScaling(t *testing.T) {
	for _, count := range []int{1000, 10000, 100000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			// Fill the catalog with descriptors for idle readable packs.
			store := NewPackfileStore(func(id string, size int64) (*PackReader, error) {
				return NewPackReader(id, size, &bytesTransport{data: []byte("pack")}), nil
			}, nil)
			t.Cleanup(store.Close)
			entries := make([]*packfile.PackfileEntry, count)
			for n := range count {
				entries[n] = &packfile.PackfileEntry{Id: "pack-" + strconv.Itoa(n), Sequence: uint64(n + 1), SizeBytes: 100, BlockCount: 1}
			}
			store.UpdateManifest(entries)

			// Retain every reader through the store's production acquisition path.
			for _, entry := range entries {
				_, release, err := store.getOrOpenEngine(entry.GetId(), 100, 1)
				if err != nil {
					t.Fatal(err)
				}
				release()
			}
			kept := store.engines["pack-0"]

			// Measure a single-pack insertion without reading or scanning any reader.
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			started := time.Now()
			store.ApplyManifestDelta([]*packfile.PackfileEntry{{Id: "new-pack", SizeBytes: 100, BlockCount: 1}}, nil)
			elapsed := time.Since(started)
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc

			// Require unchanged readers and a bounded publication allocation.
			if len(store.engines) != count || store.engines["pack-0"] != kept {
				t.Fatal("unrelated insertion changed retained readers")
			}
			if allocated > 64<<10 {
				t.Fatalf("single-pack publication allocated %d bytes with %d readers", allocated, count)
			}
			t.Logf("retained=%d readers=%d changed=1 allocated=%d mallocs=%d elapsed=%s", count, len(store.engines), allocated, after.Mallocs-before.Mallocs, elapsed)
		})
	}
}
