package world

import "github.com/aperturerobotics/hydra/block"

// ObjectOp represents an operation applied to an object with object-specific
// logic to validate & handle the transaction contents.
type ObjectOp interface {
	// Block indicates ObjectOp implements the Block interface
	block.Block
}
