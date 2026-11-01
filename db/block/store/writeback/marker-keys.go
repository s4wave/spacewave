package block_store_writeback

import "strconv"

// MarkerKeys returns the two keys of the upload marker for m, relative to the
// write-back object store: the key that records the block by hash, whose
// value is empty, and the queue key ordered by write time, whose value is
// MarkerQueueValue(m). A tool that copies a block into a volume writes both
// keys, under the object store's prefix, so the uploader finds the block.
func MarkerKeys(m Mark) (marked, queue []byte) {
	return markedKey(m.Hash), queueKey(queueTime(m.At), m.Hash)
}

// MarkerQueueValue returns the value of the queue key for m: the block size
// in decimal.
func MarkerQueueValue(m Mark) []byte {
	return []byte(strconv.FormatInt(m.Size, 10))
}
