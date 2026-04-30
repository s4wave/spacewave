//go:build sql_lite

package kvtx_block_iavl

import "github.com/s4wave/spacewave/db/block"

// AVLTree is not used by sql_lite builds, which construct standalone Tx values.
type AVLTree struct{}

func (t *AVLTree) setRootRef(*block.BlockRef) {}
