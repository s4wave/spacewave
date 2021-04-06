package psecho

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
)

// syncCheckList contains a list of wanted blocks that will trigger a sync
// session if found locally.
type syncCheckList struct {
	peer peer.ID
	refs []*block.BlockRef
}
