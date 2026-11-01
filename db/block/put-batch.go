package block

// PutBatchEntry describes one durable block write in a lower-layer batch.
type PutBatchEntry struct {
	// Ref is the expected content-addressed block reference.
	Ref *BlockRef
	// Data is the encoded block payload.
	Data []byte
	// Refs are outgoing block references recorded with this write.
	Refs []*BlockRef
	// RefsKnown declares Refs complete, as PutOpts.RefsKnown does.
	RefsKnown bool
	// Tombstone marks the block ref as deleted.
	Tombstone bool
	// Counted reports that the block is stored and its holders are already
	// counted, such as a page of a named World's tree that a later write put
	// back at its old ref. A store that counts shared blocks adds no count for
	// it, and its children count as if it named none. A layer that drops the
	// flag over-counts, which leaks and never frees.
	Counted bool
	// HoldsRoots reports that each block the entry names roots a tree that a
	// holder outside the batch already holds, such as a World a replay hold
	// names, which its lineage holds. A store that counts shared blocks counts
	// the entry's edge to a named block the batch stores afresh too, since the
	// outside holder takes that put's first holder. A layer that drops the
	// flag under-counts, which frees a live block.
	HoldsRoots bool
}
