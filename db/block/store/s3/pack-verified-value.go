//go:build !tinygo

package block_store_s3

import "github.com/s4wave/spacewave/net/hash"

// VerifiedValue is one value of a pack whose bytes were fetched from the
// bucket and hashed against the block key the pack's own index names.
type VerifiedValue struct {
	// Hash is the block hash.
	Hash *hash.Hash
	// Size is the length of the block's data.
	Size int64
}
