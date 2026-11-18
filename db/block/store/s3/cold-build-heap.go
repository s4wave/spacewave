//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"container/heap"
	"crypto/sha256"
	"os"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
)

// coldBuildCursor scans one pack's immutable index in sorted key order.
type coldBuildCursor struct {
	// pack supplies the verified bounds and the original bloom filter.
	pack *coldPack
	// reader owns the active raw tail bytes.
	reader *kvfile.Reader
	// entry is the current, not yet matched index entry.
	entry *kvfile.IndexEntry
	// pos numbers the current entry in its pack.
	pos uint64
}

// openColdBuildCursor reopens a tail and detects a change since inspection.
func openColdBuildCursor(root *os.Root, pack *coldPack) (*coldBuildCursor, error) {
	// Verify the local mirror did not change between the two passes.
	f, err := root.Open(pack.key)
	if err != nil {
		return nil, err
	}
	defer f.Close() // Read-only pack handle.

	// Rebuild the index only from the authenticated tail.
	tail, err := readLocalColdTail(f, pack.meta)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(tail)
	if !bytes.Equal(digest[:], pack.meta.IndexSha256) {
		return nil, errors.New("pack index changed while building cold manifest")
	}
	reader, err := kvfile.BuildReaderWithIndexTail(tail, pack.meta.Entry.SizeBytes)
	if err != nil {
		return nil, err
	}

	// Start the merge at the pack's first key.
	entry, err := reader.ReadIndexEntry(0)
	if err != nil {
		return nil, err
	}
	return &coldBuildCursor{pack: pack, reader: reader, entry: entry}, nil
}

// coldBuildHeap orders the active local pack cursors by their next key.
type coldBuildHeap []*coldBuildCursor

// Len reports the number of active cursors.
func (h coldBuildHeap) Len() int { return len(h) }

// Less orders the smallest remaining key first.
func (h coldBuildHeap) Less(i, j int) bool { return bytes.Compare(h[i].entry.Key, h[j].entry.Key) < 0 }

// Swap reorders two cursors.
func (h coldBuildHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push adds a cursor with an unconsumed key.
func (h *coldBuildHeap) Push(item any) { *h = append(*h, item.(*coldBuildCursor)) }

// Pop drops an exhausted cursor and releases its tail.
func (h *coldBuildHeap) Pop() any {
	// Clear the slot before shrinking so the raw tail can be collected.
	n := len(*h) - 1
	cursor := (*h)[n]
	(*h)[n] = nil
	*h = (*h)[:n]
	return cursor
}

// consume confirms the current key's bloom membership and advances its index.
func (h *coldBuildHeap) consume() error {
	// A full export key can authorize presence only if runtime lookup will
	// route it and the writer's original bloom filter recognizes it.
	c := (*h)[0]
	key := c.entry.Key
	meta := c.pack.meta
	if bytes.Compare(key, meta.FirstKey) < 0 || bytes.Compare(key, meta.LastKey) > 0 || !c.pack.filter.Test(key) {
		return errors.Errorf("cold pack %q bounds or bloom reject its index key %x", meta.Entry.Id, key)
	}

	// Move the cursor forward or release the completed pack tail.
	c.pos++
	if c.pos == c.reader.Size() {
		heap.Pop(h)
		return nil
	}
	entry, err := c.reader.ReadIndexEntry(c.pos)
	if err != nil {
		return err
	}
	if bytes.Compare(key, entry.Key) >= 0 {
		return errors.Errorf("cold pack %q index keys are not strictly ordered", meta.Entry.Id)
	}
	c.entry = entry
	heap.Fix(h, 0)
	return nil
}

// _ is a type assertion.
var _ heap.Interface = (*coldBuildHeap)(nil)
