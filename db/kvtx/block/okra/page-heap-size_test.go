package kvtx_block_okra

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// TestPageDecodedHeapSize checks the estimate against the heap that decoded
// copies of a page retain, so the decoded-block cache budget stays in bytes.
func TestPageDecodedHeapSize(t *testing.T) {
	// Build a page mixing child refs, value refs and inline blobs.
	page := &Page{LowerBound: make([]byte, 40), PageHash: make([]byte, 32)}
	for i := range 64 {
		entry := &Entry{Key: fmt.Appendf(nil, "objects/%032d", i), Hash: make([]byte, 32), Size: 1}
		switch i % 3 {
		case 0:
			entry.ChildRef = &block.BlockRef{Hash: &hash.Hash{HashType: 1, Hash: make([]byte, 32)}}
		case 1:
			entry.ValueRef = &block.BlockRef{Hash: &hash.Hash{HashType: 1, Hash: make([]byte, 32)}}
		default:
			entry.ValueIsBlob = true
			entry.ValueBlob = &blob.Blob{TotalSize: 24, RawData: make([]byte, 24)}
		}
		page.Entries = append(page.Entries, entry)
	}

	// Record the heap before any copies exist.
	const copies = 1000
	retained := make([]*Page, copies)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	// Measure the heap retained by many independent decoded copies.
	for i := range retained {
		retained[i] = page.CloneVT()
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	measured := float64(after.HeapAlloc-before.HeapAlloc) / copies
	runtime.KeepAlive(retained)

	// Compare the estimate with the measured size.
	estimate := float64(page.DecodedHeapSize())
	if ratio := estimate / measured; ratio < 0.9 || ratio > 1.1 {
		t.Fatalf("estimate %.0f bytes, measured %.0f bytes per page", estimate, measured)
	}
}
