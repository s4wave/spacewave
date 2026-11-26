package block_gc

import "github.com/s4wave/spacewave/db/block"

// MarkedListSample names a marked list the rebuild found wanting.
type MarkedListSample struct {
	// Ref is the block.
	Ref *block.BlockRef
	// Type names the type the walker read the block as.
	Type string
	// Missing is the first child the content names that the list lacks, nil
	// for an undecodable block.
	Missing *block.BlockRef
}
