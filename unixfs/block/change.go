package unixfs_block

import "github.com/aperturerobotics/hydra/block"

// IsNil returns if the object is nil.
func (c *FSChange) IsNil() bool {
	return c == nil
}

// _ is a type assertion
var _ block.SubBlock = ((*FSChange)(nil))
