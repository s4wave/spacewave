// Package block defines a common pattern for interacting with block reference
// structures in Hydra and in memory.
package block

import (
	"context"
	"errors"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"gonum.org/v1/gonum/graph/encoding"
)

// MaxBlockSize is the default maximum size in bytes (10MB) for a single
// serialized block accepted on the wire by transports such as DEX.
//
// This is a sanity / DoS ceiling, not a structural limit on any one block
// type. Individual block types are naturally bounded well below this value:
//
//   - blob.Blob (root): inline RawData is capped by
//     blob.DefRawHighWaterMark = blob.DefChunkingMaxSize = 786432 (768 KiB).
//     Above that the Blob auto-converts to CHUNKED, where the root only
//     stores a ChunkIndex of references and individual chunk data lives in
//     separate byteslice blocks.
//   - blob chunk data (byteslice.ByteSlice): one chunk per block, capped by
//     blob.DefChunkingMaxSize = 786432 (768 KiB).
//   - blob.ChunkIndex / IAVL node / other index blocks: hold only references
//     and small metadata, well under 1 MiB in practice.
//
// 10 MiB therefore leaves roughly an order of magnitude of headroom over the
// largest block any current producer will emit, while still bounding the
// per-message buffer a peer must allocate when receiving a single block.
const MaxBlockSize = 10485760

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

// ComparableSubBlock is the type constraint for SubBlock.
type ComparableSubBlock interface {
	comparable
	SubBlock
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

// NewSubBlockCtor constructs a new SubBlock constructor.
// returns nil if r is nil
// usage: block.NewSubBlockCtor(r, func() *ChangeLogLL { return &ChangeLogLL{} })
func NewSubBlockCtor[T ComparableSubBlock](r *T, ctor func() T) SubBlockCtor {
	if r == nil {
		return nil
	}
	var empty T
	return func(create bool) SubBlock {
		v := *r
		if create && v == empty {
			v = ctor()
			*r = v
		}
		return v
	}
}

// ApplySubBlock applies a sub-block to a field.
func ApplySubBlock[T SubBlock](r *T, next SubBlock) error {
	if r == nil {
		return errors.New("apply sub block: pointer to target cannot be nil")
	}
	v, ok := next.(T)
	if !ok {
		return ErrUnexpectedType
	}
	*r = v
	return nil
}

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

// CastToBlock casts a object to a block or returns an error.
func CastToBlock(sb any) (Block, error) {
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
func CloneBlock(blk any) (any, error) {
	if blk == nil {
		return nil, nil
	}

	switch og := blk.(type) {
	case BlockWithClone:
		return og.CloneBlock()
	case protobuf_go_lite.CloneMessage:
		obm := og.CloneMessageVT()
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
func UnmarshalBlock[T Block](ctx context.Context, bcs *Cursor, ctor func() Block) (T, error) {
	var out T
	if bcs == nil {
		return out, nil
	}
	blk, err := bcs.Unmarshal(ctx, ctor)
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
