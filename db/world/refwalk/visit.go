package refwalk

import "github.com/s4wave/spacewave/db/block"

// Visit is what reading one block found.
type Visit struct {
	// Node is the block read.
	Node Node
	// Found is false when the store lacks the block.
	Found bool
	// Leaf is true when the block's type holds no refs, so only its presence
	// was read, or its bytes when the walker keeps them.
	Leaf bool
	// DecodeErr is why the block did not decode as its type.
	DecodeErr error
	// Refs are the refs a write records for the decoded block, nil for a leaf.
	Refs []*block.BlockRef
	// Data is the block as stored, before any transform decodes it, when the
	// walker keeps bytes and the block was found; nil otherwise.
	Data []byte
	// blk is the decoded block, nil for a leaf or an undecodable block.
	blk any
}
