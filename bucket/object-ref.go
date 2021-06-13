package bucket

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/transform"
	"github.com/golang/protobuf/proto"
	b58 "github.com/mr-tron/base58/base58"
)

// NewObjectRefBlock constructs a new object ref block.
func NewObjectRefBlock() block.Block {
	return &ObjectRef{}
}

// NewObjectRefSubBlock constructs a new object ref sub-block constructor.
func NewObjectRefSubBlockCtor(r **ObjectRef) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if v == nil && create {
			v = &ObjectRef{}
			*r = v
		}
		return v
	}
}

// ParseObjectRef parses an object ref string.
func ParseObjectRef(ref string) (*ObjectRef, error) {
	if ref == "" {
		return nil, nil
	}

	dat, err := b58.Decode(ref)
	if err != nil {
		return nil, err
	}
	o := &ObjectRef{}
	if err := proto.Unmarshal(dat, o); err != nil {
		return nil, err
	}
	return o, nil
}

// Validate performs cursory validation of the object ref.
func (o *ObjectRef) Validate() error {
	if err := o.GetRootRef().Validate(); err != nil {
		return err
	}
	if !o.GetTransformConfRef().GetEmpty() {
		if err := o.GetTransformConfRef().Validate(); err != nil {
			return err
		}
	}
	if len(o.GetTransformConf().GetSteps()) != 0 {
		if err := o.GetTransformConf().Validate(); err != nil {
			return err
		}
	}
	return nil
}

// GetEmpty returns if the ref and transform configs are empty.
func (b *ObjectRef) GetEmpty() bool {
	return b.GetRootRef().GetEmpty() &&
		b.GetBucketId() == "" &&
		b.GetTransformConfRef().GetEmpty() &&
		b.GetTransformConf().GetEmpty()
}

// EqualsRef checks if the ref is equal to another ref.
func (b *ObjectRef) EqualsRef(ot *ObjectRef) bool {
	if b == nil && ot == nil {
		return true
	}

	switch {
	case b.GetEmpty() != ot.GetEmpty():
	case !b.GetRootRef().EqualsRef(ot.GetRootRef()):
	case (b.GetTransformConf() == nil) != (ot.GetTransformConf() == nil):
	case !proto.Equal(b.GetTransformConf(), ot.GetTransformConf()):
	case b.GetTransformConfRef().GetEmpty() != ot.GetTransformConfRef().GetEmpty():
	case !b.GetTransformConfRef().EqualsRef(ot.GetTransformConfRef()):
	case b.GetBucketId() != ot.GetBucketId():
	default:
		return true
	}

	return false
}

// MarshalString marshals the reference to a string form.
func (b *ObjectRef) MarshalString() string {
	if b == nil {
		return ""
	}
	dat, err := proto.Marshal(b)
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *ObjectRef) MarshalBlock() ([]byte, error) {
	return proto.Marshal(b)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *ObjectRef) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, b)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (b *ObjectRef) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		b.RootRef = ptr
	case 3:
		b.TransformConfRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (b *ObjectRef) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, 2)
	// ignore root-ref if bucket id is not empty
	if len(b.GetBucketId()) == 0 {
		m[1] = b.GetRootRef()
	}
	m[3] = b.GetTransformConfRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (b *ObjectRef) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		// unknown
	case 3:
		return block_transform.NewTransformConfigBlock
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block = ((*ObjectRef)(nil))
	// note: only refs with zero-len bucket id are returned
	_ block.BlockWithRefs = ((*ObjectRef)(nil))
)
