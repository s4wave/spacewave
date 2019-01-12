// Block defines a common pattern for interacting with block reference
// structures in Hydra and in memory.
package block

import (
	"github.com/aperturerobotics/hydra/cid"
)

// Block defines an in-memory decoded block structure. A block should contain a
// minimal amount of data with some pointers to other blocks.
type Block interface {
	// MarshalBlock marshals the block to binary.
	// This is the initial step of marshaling, before transformations.
	MarshalBlock() ([]byte, error)
	// UnmarshalBlock unmarshals the block to the object.
	// This is the final step of decoding, after transformations.
	UnmarshalBlock(data []byte) error
	// ApplyRef applies a ref change with a field id.
	// The reference may be nil if the child block is nil.
	ApplyRef(id uint32, ptr *cid.BlockRef) error
}
