package world_block

import (
	"github.com/s4wave/spacewave/db/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// GraphHoleFill is what an older World held in the key range of one hole.
type GraphHoleFill struct {
	// Root is the World root the keys came from, or nil when no root held
	// the whole range.
	Root *block.BlockRef
	// Keys is the number of keys the range held in Root.
	Keys int
	// Tried are the roots that did not hold the whole range, with why.
	Tried []string
	// entries are the keys of the range.
	entries []kvtx_block_okra.BuildEntry
}
