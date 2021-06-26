package kvtx_block_iavl

import "errors"

// ErrMustBeBlock is returned if a cursor is not a block
var ErrMustBeBlock = errors.New("iavl value sub-block must implement block interface")
