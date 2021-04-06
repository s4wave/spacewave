package psecho

import (
	"github.com/aperturerobotics/hydra/block"
)

// rxBlock contains a block coming from a remote peer
// the block must be hashed before being accepted
type rxBlock struct {
	ref  *block.BlockRef
	data []byte
}
