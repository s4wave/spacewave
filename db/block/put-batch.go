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
	// External marks a block whose bytes the caller keeps in a store of its
	// own, such as a local packahead Log. A Volume that keeps liveness lands
	// its child list and holders without storing the bytes, so it holds the
	// block as it holds an evicted one: a read of the bytes misses, and a later
	// put without External stores them. A store that keeps no liveness stores
	// the bytes as for any entry, which loses nothing.
	External bool
}
