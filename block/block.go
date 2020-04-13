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
}

// BlockWithRefs has references keyed by ID.
// Each field can contain a reference.
type BlockWithRefs interface {
	// ApplyBlockRef applies a ref change with a field id.
	// The reference may be nil if the child block is nil.
	ApplyBlockRef(id uint32, ptr *cid.BlockRef) error
	// GetBlockRefs returns all block references by ID.
	// May return nil, and values may also be nil.
	// Note: this does not include pending references (in a cursor)
	GetBlockRefs() (map[uint32]*cid.BlockRef, error)
	// GetBlockRefCtor returns the constructor for the block at the ref id.
	// Return nil to indicate invalid ref ID or unknown.
	GetBlockRefCtor(id uint32) Ctor
}

// SubBlock is a object contained inside a Block.
// May optionally implement Block or other Block interfaces.
type SubBlock interface{}

// SubBlockCtor constructs a sub-block.
// If create == false, returns nil if the field is not set.
type SubBlockCtor func(create bool) SubBlock

// BlockWithSubBlocks is a block containing sub-blocks as fields.
type BlockWithSubBlocks interface {
	// ApplySubBlock applies a sub-block change with a field id.
	ApplySubBlock(id uint32, next SubBlock) error
	// GetSubBlocks returns all constructed sub-blocks by ID.
	// May return nil, and values may also be nil.
	GetSubBlocks() map[uint32]SubBlock
	// GetSubBlockCtor returns a function which creates or returns the existing
	// sub-block at reference id. Can return nil to indicate invalid reference id.
	GetSubBlockCtor(id uint32) SubBlockCtor
}

// BlockWithAttributes returns a block with graph attributes.
type BlockWithAttributes interface {
	// GetBlockGraphAttributes returns the block graph attributes.
	GetBlockGraphAttributes() []encoding.Attribute
}
