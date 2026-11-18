package provider_local

import (
	"bytes"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// TestPlacedColdPackFallback checks the placed read order and the boundary
// that makes only legacy SQL reads write migration markers.
func TestPlacedColdPackFallback(t *testing.T) {
	// Keep the cold reader's bytes in memory; the fixture uses the real placed
	// chains and write-back store, and an SQL stand-in for the legacy fallback.
	f := newFallbackTest(t)
	hot := newLeafStore()
	cold := &countingStore{StoreOps: newLeafStore()}
	f.p.cold = cold
	remote := func() block.StoreOps { return hot }
	f.reads, f.copies = f.p.readChains(f.local, f.local, remote, nil, f.sql)
	serve := f.p.serveReads(f.local, remote, f.sql)

	// Put one block in both pack tiers, and another in cold packs and SQL.
	hotData, coldData, sqlData := []byte("hot and cold"), []byte("cold and SQL"), []byte("only SQL")
	opts := &block.PutOpts{RefsKnown: true}
	hotRef, _, err := hot.PutBlock(f.ctx, hotData, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cold.PutBlock(f.ctx, hotData, opts); err != nil {
		t.Fatal(err)
	}
	coldRef := f.putSQL(coldData)
	if _, _, err := cold.PutBlock(f.ctx, coldData, opts); err != nil {
		t.Fatal(err)
	}
	sqlRef := f.putSQL(sqlData)

	// A hot hit takes precedence over a copy in the cold set.
	f.read(hotRef, hotData)
	if got := cold.reads.Load(); got != 0 {
		t.Fatalf("hot hit made %d cold reads", got)
	}

	// Cold hits serve data and stat, including on scoped copies and peer serves.
	for name, store := range map[string]block.StoreOps{"read": f.reads, "copy": f.copies, "serve": serve} {
		t.Run(name, func(t *testing.T) {
			// Exercise the entries through the acquired read scope.
			scoped, release, err := store.BeginReadOperation(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			// Require consistent data, presence and stat from the cold reader.
			stored, err := scoped.GetStoredBlock(t.Context(), coldRef)
			if err != nil || !bytes.Equal(stored.GetData(), coldData) {
				t.Fatalf("cold stored read = %v, %v", stored, err)
			}
			found, err := scoped.GetBlockExists(t.Context(), coldRef)
			if err != nil || !found {
				t.Fatalf("cold existence = %t, %v", found, err)
			}
			stat, err := scoped.StatBlock(t.Context(), coldRef)
			if err != nil || stat == nil || stat.Size != int64(len(coldData)) {
				t.Fatalf("cold stat = %v, %v", stat, err)
			}
		})
	}

	// A cold hit must not consult SQL, fill the disk or mark an upload.
	if f.sql.reads.Load() != 0 || f.p.fallbackFills.status().Hits != 0 || f.held(coldRef) || f.status().Pending != 0 {
		t.Fatalf("cold hit touched SQL or filled the local store (SQL reads %d; fills %+v)", f.sql.reads.Load(), f.p.fallbackFills.status())
	}

	// A batch combines the cold, hot and SQL answers in the request's order.
	missing, err := block.BuildBlockRef([]byte("not present"), opts)
	if err != nil {
		t.Fatal(err)
	}
	found, err := f.reads.GetBlockExistsBatch(f.ctx, []*block.BlockRef{coldRef, missing, sqlRef, hotRef})
	if err != nil || !slices.Equal(found, []bool{true, false, true, true}) {
		t.Fatalf("mixed existence batch = %v, %v", found, err)
	}

	// Only a real SQL fallback hit fills and marks the block for upload.
	f.read(sqlRef, sqlData)
	f.waitPending(1)
	if !f.held(sqlRef) || f.held(coldRef) || f.p.fallbackFills.status().Hits != 1 {
		t.Fatalf("fallback fills = %+v; want only the SQL hit filled", f.p.fallbackFills.status())
	}
}

// TestColdPackPrefixIsolation keeps the cold set outside even full hot-store
// deletion, which lists everything under the hot store's prefix.
func TestColdPackPrefixIsolation(t *testing.T) {
	// Reject nesting in either direction, including a normalized hot path.
	for _, test := range []struct {
		cold string
		hot  string
	}{
		{"world/space/", "world/space/"},
		{"world/space/packs/", "world/space/"},
		{"world/space/entries/", "world/space/"},
		{"world/space/cold/", "world/space/"},
		{"world/", "world/space/"},
		{"world/space/", "world/./space/"},
	} {
		if err := validateColdPackPrefix(test.cold, test.hot); err == nil {
			t.Errorf("accepted cold prefix %q against hot prefix %q", test.cold, test.hot)
		}
	}

	// A prefix with similar words but no shared directory is safe.
	if err := validateColdPackPrefix("world/space-cold/", "world/space/"); err != nil {
		t.Fatal(err)
	}
}
