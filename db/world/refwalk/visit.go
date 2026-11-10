package refwalk

import "github.com/s4wave/spacewave/db/block"

// Visit is what reading one block found.
type Visit struct {
	// Node is the block read.
	Node Node
	// Found is false when the store lacks the block.
	Found bool
	// Leaf is true when the block's type holds no refs, so only its presence
	// was read.
	Leaf bool
	// DecodeErr is why the block did not decode as its type.
	DecodeErr error
	// Refs are the refs a write records for the decoded block, nil for a leaf.
	Refs []*block.BlockRef
	// blk is the decoded block, nil for a leaf or an undecodable block.
	blk any
}
