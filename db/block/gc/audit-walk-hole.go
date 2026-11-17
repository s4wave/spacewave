package block_gc

import (
	"github.com/s4wave/spacewave/db/block"
)

// UnconfirmedHole is a candidate still named and absent once the confirm
// delay passed.
type UnconfirmedHole struct {
	// Ref is the block.
	Ref *block.BlockRef
	// Class says which roots reach it.
	Class HoleClass
}
