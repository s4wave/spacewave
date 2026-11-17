package block_store_writeback

import "github.com/s4wave/spacewave/net/hash"

// EvictVerifiedValue is a value of a pack whose fetched bytes hashed to its
// key.
type EvictVerifiedValue struct {
	// Hash is the block hash.
	Hash *hash.Hash
	// Size is the length of the block's data.
	Size int64
}
