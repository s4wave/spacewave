package psecho

import (
	"github.com/aperturerobotics/hydra/cid"
)

// rxBlock contains a block coming from a remote peer
// the block must be hashed before being accepted
type rxBlock struct {
	ref  *cid.BlockRef
	data []byte
}
