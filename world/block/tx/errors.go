package world_block_tx

import "errors"

var (
	// ErrLimitedOps is returned if an unsupported op is used.
	ErrLimitedOps = errors.New("block tx is limited to applying world/object batch operations only")
)
