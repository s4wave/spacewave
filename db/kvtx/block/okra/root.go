package kvtx_block_okra

import "github.com/s4wave/spacewave/db/block"

// HashSize is the Okra K constant in bytes.
const HashSize = 16

// FanoutDegree is the Okra Q promotion constant.
const FanoutDegree = 32

const rootPageRefID = 4

// NewRootBlock constructs a new Okra root block.
func NewRootBlock() block.Block {
	return &Root{}
}

// NewRootSubBlockCtor returns the sub-block constructor.
func NewRootSubBlockCtor(r **Root) block.SubBlockCtor {
	return block.NewSubBlockCtor(r, func() *Root { return &Root{} })
}

// IsNil checks if the object is nil.
func (r *Root) IsNil() bool {
	return r == nil
}

// Validate performs cursory checks of the Okra root object.
func (r *Root) Validate() error {
	if r.GetSize() == 0 {
		if r.GetHeight() != 0 || len(r.GetRootHash()) != 0 ||
			!r.GetRootPageRef().GetEmpty() ||
			r.GetHashSize() != 0 || r.GetFanoutDegree() != 0 {
			return ErrUnexpectedEmptyRootHeight
		}
		return nil
	}
	if r.GetHeight() == 0 ||
		len(r.GetRootHash()) != HashSize ||
		r.GetHashSize() != HashSize ||
		r.GetFanoutDegree() != FanoutDegree {
		return ErrUnexpectedRootMetadata
	}
	if err := r.GetRootPageRef().Validate(false); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *Root) MarshalBlock() ([]byte, error) {
	return r.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *Root) UnmarshalBlock(data []byte) error {
	return r.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
func (r *Root) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case rootPageRefID:
		r.RootPageRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
func (r *Root) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if r.GetSize() == 0 {
		return nil, nil
	}
	return map[uint32]*block.BlockRef{
		rootPageRefID: r.GetRootPageRef(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
func (r *Root) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case rootPageRefID:
		return NewPageBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block         = (*Root)(nil)
	_ block.SubBlock      = (*Root)(nil)
	_ block.BlockWithRefs = (*Root)(nil)
)
