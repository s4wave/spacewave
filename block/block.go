// Package block defines a common pattern for interacting with block reference
// structures in Hydra and in memory.
package block

import (
	"gonum.org/v1/gonum/graph/encoding"
	proto "google.golang.org/protobuf/proto"
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
	ApplyBlockRef(id uint32, ptr *BlockRef) error
	// GetBlockRefs returns all block references by ID.
	// May return nil, and values may also be nil.
	// Note: this does not include pending references (in a cursor)
	GetBlockRefs() (map[uint32]*BlockRef, error)
	// GetBlockRefCtor returns the constructor for the block at the ref id.
	// Return nil to indicate invalid ref ID or unknown.
	GetBlockRefCtor(id uint32) Ctor
}

// SubBlock is a object contained inside a Block.
// May optionally implement Block or other Block interfaces.
type SubBlock interface {
	// IsNil checks if the object is nil.
	IsNil() bool
}

// NamedSubBlock is a sub-block with a name attached.
type NamedSubBlock interface {
	// SubBlock indicates this is a sub-block.
	SubBlock
	// GetName returns the name of the block.
	GetName() string
}

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

// BlockWithPreWriteHook is a block with a function called when writing.
// This can also be applied to a sub-block.
type BlockWithPreWriteHook interface {
	// BlockPreWriteHook is called when writing the block.
	BlockPreWriteHook() error
}

// BlockWithClone defines a block with a clone function.
// The clone should share nothing with the original.
type BlockWithClone interface {
	CloneBlock() (Block, error)
}

// BlockWithCloneVT defines a block with a VTProtobuf clone function.
type BlockWithCloneVT[T Block] interface {
	// CloneVT clones the block object with VTprotobuf.
	CloneVT() T
}

// Validate validates the put opts.
func (o *PutOpts) Validate() error {
	if o == nil {
		return nil
	}
	if o.GetHashType() != 0 {
		if err := o.GetHashType().Validate(); err != nil {
			return err
		}
	}
	return nil
}

// CastToBlock casts a object to a block or returns an error.
func CastToBlock(sb interface{}) (Block, error) {
	if sb == nil {
		return nil, nil
	}

	b, blkOk := sb.(Block)
	if !blkOk {
		return nil, ErrNotBlock
	}
	return b, nil
}

// CloneBlock tries to clone an input block.
//
// The block should implement proto.Message or BlockWithClone.
//
// returns ErrUnexpectedType or ErrNotClonable if the block was not clonable.
func CloneBlock(blk interface{}) (interface{}, error) {
	if blk == nil {
		return nil, nil
	}

	switch og := blk.(type) {
	case BlockWithClone:
		return og.CloneBlock()
	case proto.Message:
		obm := proto.Clone(og)
		ob, ok := obm.(Block)
		if !ok {
			return nil, ErrUnexpectedType
		}
		return ob, nil
	}

	return nil, ErrNotClonable
}

// UnmarshalBlock unmarshals the block from the cursor & type-asserts it.
// Returns ErrUnexpectedType if the type returned was not T.
// Incorrect type happens if the cursor already contains a block w/ different type.
// If bcs == nil, returns empty, nil.
// If unmarshal() returns nil, returns empty, nil.
func UnmarshalBlock[T Block](bcs *Cursor, ctor func() Block) (T, error) {
	var out T
	if bcs == nil {
		return out, nil
	}
	blk, err := bcs.Unmarshal(ctor)
	if err != nil {
		return out, err
	}
	if blk == nil {
		return out, nil
	}
	var ok bool
	out, ok = blk.(T)
	if !ok {
		return out, ErrUnexpectedType
	}
	return out, nil
}
