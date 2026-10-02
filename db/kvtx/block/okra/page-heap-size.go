package kvtx_block_okra

import (
	"unsafe"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
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
	n := heapAlloc(int(unsafe.Sizeof(*p))) +
		heapBytes(p.LowerBound) +
		heapBytes(p.UpperBound) +
		heapBytes(p.PageHash) +
		heapBytes(p.unknownFields) +
		heapAlloc(cap(p.Entries)*int(unsafe.Sizeof((*Entry)(nil))))
	for _, entry := range p.Entries {
		n += entryHeapSize(entry)
	}
	return n
}

// entryHeapSize estimates the heap bytes one entry retains.
func entryHeapSize(e *Entry) int {
	if e == nil {
		return 0
	}
	return heapAlloc(int(unsafe.Sizeof(*e))) +
		heapBytes(e.Key) +
		heapBytes(e.Hash) +
		heapBytes(e.unknownFields) +
		blockRefHeapSize(e.ChildRef) +
		blockRefHeapSize(e.ValueRef) +
		blobHeapSize(e.ValueBlob)
}

// blockRefHeapSize estimates the heap bytes a block ref retains.
func blockRefHeapSize(ref *block.BlockRef) int {
	if ref == nil {
		return 0
	}
	n := heapAlloc(int(unsafe.Sizeof(*ref)))
	if h := ref.GetHash(); h != nil {
		n += heapAlloc(int(unsafe.Sizeof(hash.Hash{}))) + heapBytes(h.GetHash())
	}
	return n
}

// blobHeapSize estimates the heap bytes an inline blob retains. A chunk index
// is charged its encoded size.
func blobHeapSize(b *blob.Blob) int {
	if b == nil {
		return 0
	}
	return heapAlloc(int(unsafe.Sizeof(*b))) +
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
