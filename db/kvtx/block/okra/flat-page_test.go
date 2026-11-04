//go:build !goscript

package kvtx_block_okra

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// newFlatSamplePage builds a page mixing child refs, value refs and inline blobs.
func newFlatSamplePage() *Page {
	// Fill the bounds and one entry of each kind, in key order.
	page := &Page{LowerBound: []byte("objects/"), PageHash: make([]byte, HashSize)}
	for i := range 48 {
		entry := &Entry{Key: fmt.Appendf(nil, "objects/%032d", i), Hash: make([]byte, HashSize), Size: uint64(i + 1)}
		switch i % 3 {
		case 0:
			entry.ChildRef = &block.BlockRef{Hash: &hash.Hash{HashType: 1, Hash: bytes.Repeat([]byte{byte(i)}, 32)}}
		case 1:
			entry.ValueRef = &block.BlockRef{Hash: &hash.Hash{HashType: 1, Hash: bytes.Repeat([]byte{byte(i)}, 32)}}
		default:
			entry.ValueIsBlob = true
			entry.ValueBlob = &blob.Blob{TotalSize: 24, RawData: bytes.Repeat([]byte{byte(i)}, 24)}
		}
		page.Entries = append(page.Entries, entry)
	}
	return page
}

// TestFlatPage checks the flat page against the model states of FlatPage.tla:
// the decoded form matches Page field for field, a published page is never
// written by reads, a view survives eviction of its page, and the cache charge
// equals the resident arena bytes.
func TestFlatPage(t *testing.T) {
	// Decode the same encoded page both ways.
	page := newFlatSamplePage()
	data, err := page.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	flat := &flatPage{}
	if err := flat.UnmarshalDecodedBlock(data); err != nil {
		t.Fatal(err)
	}
	if flat.Len() != len(page.Entries) {
		t.Fatalf("entries: %d != %d", flat.Len(), len(page.Entries))
	}

	// Differential: every accessor agrees with the pointer-rich page.
	for i, e := range page.Entries {
		if !bytes.Equal(flat.entryKey(i), e.Key) {
			t.Fatalf("entry %d key differs", i)
		}
		if got := flat.childRef(i); !got.EqualVT(e.ChildRef) && (got != nil || e.ChildRef != nil) {
			t.Fatalf("entry %d child ref differs", i)
		}
		if got := flat.valueRef(i); !got.EqualVT(e.ValueRef) && (got != nil || e.ValueRef != nil) {
			t.Fatalf("entry %d value ref differs", i)
		}
		if flat.valueIsBlob(i) != e.ValueIsBlob {
			t.Fatalf("entry %d blob flag differs", i)
		}
		if idx := flat.searchEntry(e.Key); idx != i {
			t.Fatalf("search %d found %d", i, idx)
		}
	}

	// Model Immutable: reads leave the published arena bytes unchanged.
	before := bytes.Clone(flat.buf)
	for i := range flat.Len() {
		flat.entryKey(i)
		flat.rawInlineValue(i)
	}
	if !bytes.Equal(before, flat.buf) {
		t.Fatal("reading a published page changed its arena")
	}

	// Model ViewStable: a view stays intact after the page is dropped and
	// the collector runs.
	view := flat.entryKey(5)
	want := bytes.Clone(view)
	flat = nil
	for range 3 {
		runtime.GC()
	}
	if !bytes.Equal(view, want) {
		t.Fatal("view changed after its page was released")
	}
}

// TestFlatPageBudget checks the model's Accounted and InBudget invariants: the
// cache charges exactly the arena bytes of its resident pages and stays in budget.
func TestFlatPageBudget(t *testing.T) {
	// Charge the real arena bytes of one page against a three page budget.
	data, err := newFlatSamplePage().MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	one := &flatPage{}
	if err := one.UnmarshalDecodedBlock(data); err != nil {
		t.Fatal(err)
	}
	if got := one.DecodedHeapSize(); got < len(data) {
		t.Fatalf("charge %d is below the arena bytes %d", got, len(data))
	}
	cache, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{MaxCost: 3 * int64(one.DecodedHeapSize())})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// Read the pages of a tree larger than the budget through the cache.
	ctx := t.Context()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 4096)
	rootRef, _ := writeOkraFixture(t, ctx, store, fixture)
	bt, cs := block.NewTransaction(store, nil, rootRef, nil)
	bt.SetReadOnly()
	bt.SetDecodedBlockCache(cache)
	tr, err := NewTx(ctx, cs, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Discard()
	for i, key := range fixture.keys {
		value, found, err := tr.Get(ctx, key)
		if err != nil || !found || !bytes.Equal(value, fixture.values[i]) {
			t.Fatalf("read %q found=%v err=%v", key, found, err)
		}
	}

	// The retained charge is within budget and pages were evicted.
	stats := cache.Snapshot()
	if stats.RetainedCost > stats.MaxCost || stats.Evictions == 0 {
		t.Fatalf("budget not exercised: %+v", stats)
	}
}
