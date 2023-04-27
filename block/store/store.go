package block_store

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/sirupsen/logrus"
)

// Store can read/write blocks.
type Store = block.Store

// ErrReadOnlyStore is returned if the block store is not writable.
var ErrReadOnlyStore = errors.New("block store is read-only")

// Constructor constructs a block store with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
) (Store, error)
