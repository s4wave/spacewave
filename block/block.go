// Package block defines a common pattern for interacting with block reference
// structures in Hydra and in memory.
package block

import (
	"github.com/aperturerobotics/hydra/cid"
	"gonum.org/v1/gonum/graph/encoding"
)

// Ctor is a block constructor.
type Ctor func() Block

// Block defines an in-memory decoded block structure. A block should contain a
// minimal amount of data with some pointers to other blocks.
type Block interface {
	// MarshalBlock marshals the block to binary.
	// This is the initial step of marshaling, before transformations.
	MarshalBlock() ([]byte, error)
	// UnmarshalBlock unmarshals the block to the object.
	// This is the final step of decoding, after transformations.
	UnmarshalBlock(data []byte) error
	// ApplyBlockRef applies a ref change with a field id.
	// The reference may be nil if the child block is nil.
	ApplyBlockRef(id uint32, ptr *cid.BlockRef) error
	// GetBlockRefs returns all block references by ID.
	// May return nil, and values may also be nil.
	// Note: this does not include pending references (in a cursor)
	GetBlockRefs() (map[uint32]*cid.BlockRef, error)
	// GetBlockRefCtor returns the constructor for the block at the ref id.
	// Return nil to indicate invalid ref ID.
	GetBlockRefCtor(id uint32) Ctor
}

// BlockWithAttributes returns a block with graph attributes.
type BlockWithAttributes interface {
	// GetBlockGraphAttributes returns the block graph attributes.
	GetBlockGraphAttributes() []encoding.Attribute
}
