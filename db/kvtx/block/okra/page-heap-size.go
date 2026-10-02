package kvtx_block_okra

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// Heap allocation sizes of the decoded messages on 64-bit Go, rounded up to
// the allocator size class. TestPageDecodedHeapSize checks them against
// measured heap, so update them when the messages change.
const (
	pageHeapSize     = 144
	entryHeapSize    = 128
	blockRefHeapSize = 32
	hashHeapSize     = 64
	blobHeapSize     = 80
	pointerHeapSize  = 8
)

// heapAllocAlign approximates the Go allocator rounding small objects up to
// their size class.
const heapAllocAlign = 16

// DecodedHeapSize estimates the heap bytes the decoded page retains. Each
// entry holds about 120 bytes of struct fields, so a page of small entries
// retains two to three times its encoded size.
func (p *Page) DecodedHeapSize() int {
	if p == nil {
		return 0
	}
	n := pageHeapSize +
		heapBytes(p.LowerBound) +
		heapBytes(p.UpperBound) +
		heapBytes(p.PageHash) +
		heapBytes(p.unknownFields) +
		heapAlloc(cap(p.Entries)*pointerHeapSize)
	for _, entry := range p.Entries {
		n += entryDecodedHeapSize(entry)
	}
	return n
}

// entryDecodedHeapSize estimates the heap bytes one entry retains.
func entryDecodedHeapSize(e *Entry) int {
	if e == nil {
		return 0
	}
	return entryHeapSize +
		heapBytes(e.Key) +
		heapBytes(e.Hash) +
		heapBytes(e.unknownFields) +
		blockRefDecodedHeapSize(e.ChildRef) +
		blockRefDecodedHeapSize(e.ValueRef) +
		blobDecodedHeapSize(e.ValueBlob)
}

// blockRefDecodedHeapSize estimates the heap bytes a block ref retains.
func blockRefDecodedHeapSize(ref *block.BlockRef) int {
	if ref == nil {
		return 0
	}
	n := blockRefHeapSize
	if h := ref.GetHash(); h != nil {
		n += hashHeapSize + heapBytes(h.GetHash())
	}
	return n
}

// blobDecodedHeapSize estimates the heap bytes an inline blob retains. A chunk
// index is charged its encoded size.
func blobDecodedHeapSize(b *blob.Blob) int {
	if b == nil {
		return 0
	}
	return blobHeapSize +
		heapBytes(b.GetRawData()) +
		b.GetChunkIndex().SizeVT()
}

// heapBytes returns the heap bytes of a byte slice's backing array.
func heapBytes(b []byte) int {
	return heapAlloc(cap(b))
}

// heapAlloc rounds an allocation of n bytes up to heapAllocAlign.
func heapAlloc(n int) int {
	return (n + heapAllocAlign - 1) / heapAllocAlign * heapAllocAlign
}

// _ is a type assertion
var _ block.DecodedBlockHeapSizer = (*Page)(nil)
