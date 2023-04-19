package block_store

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
)

// Store can read/write blocks.
type Store = block.Store

// ErrReadOnlyStore is returned if the block store is not writable.
var ErrReadOnlyStore = errors.New("block store is read-only")
