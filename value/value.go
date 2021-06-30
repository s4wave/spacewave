package forge_value

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

// NewValueWithBlockRef constructs a new value with a block ref.
func NewValueWithBlockRef(br *block.BlockRef) *Value {
	return &Value{
		ValueType: ValueType_ValueType_BLOCK_REF,
		BlockRef:  br,
	}
}

// Validate checks the value type is in range.
func (v ValueType) Validate() error {
	switch v {
	case ValueType_ValueType_BLOCK_REF:
	case ValueType_ValueType_BUCKET_REF:
	default:
		return errors.Wrap(ErrUnknownValueType, v.String())
	}
	return nil
}

// Validate performs cursory validation of the value.
func (v *Value) Validate(allowEmptyName bool) error {
	if len(v.GetName()) == 0 && !allowEmptyName {
		return ErrEmptyValueName
	}
	vt := v.GetValueType()
	if err := vt.Validate(); err != nil {
		return nil
	}
	if vt == ValueType_ValueType_BLOCK_REF {
		if err := v.GetBlockRef().Validate(); err != nil {
			return err
		}
	} else {
		if !v.GetBlockRef().GetEmpty() {
			return errors.Errorf(
				"expect empty block_ref field for non-block-ref value type %s",
				vt.String(),
			)
		}
	}
	if vt == ValueType_ValueType_BUCKET_REF {
		if err := v.GetBucketRef().Validate(); err != nil {
			return err
		}
	} else {
		if !v.GetBucketRef().GetEmpty() {
			return errors.Errorf(
				"expect empty bucket_ref field for non-bucket-ref value type %s",
				vt.String(),
			)
		}
	}
	return nil
}

// IsEmpty checks if the configuration is empty.
func (v *Value) IsEmpty() bool {
	valueType := v.GetValueType()
	if valueType == ValueType_ValueType_UNKNOWN {
		return true
	}
	if valueType == ValueType_ValueType_BLOCK_REF {
		return v.GetBlockRef().GetEmpty()
	}
	if valueType == ValueType_ValueType_BUCKET_REF {
		return v.GetBucketRef().GetEmpty()
	}
	return true
}

// Clone deep copies the Value.
func (v *Value) Clone() *Value {
	if v == nil {
		return nil
	}
	return proto.Clone(v).(*Value)
}

// ToBucketRef converts any value type into an ObjectRef.
// Returns nil if the block ref or bucket ref was empty.
func (v *Value) ToBucketRef() (*bucket.ObjectRef, error) {
	vt := v.GetValueType()
	switch vt {
	case ValueType_ValueType_UNKNOWN:
		return nil, nil
	case ValueType_ValueType_BUCKET_REF:
		return v.GetBucketRef(), nil
	case ValueType_ValueType_BLOCK_REF:
		blockRef := v.GetBlockRef()
		if blockRef == nil {
			return nil, nil
		}
		return &bucket.ObjectRef{RootRef: v.GetBlockRef()}, nil
	default:
		return nil, errors.Wrap(ErrUnknownValueType, vt.String())
	}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (v *Value) MarshalBlock() ([]byte, error) {
	return proto.Marshal(v)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (v *Value) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, v)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (v *Value) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 3:
		v.BlockRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (v *Value) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef)
	m[3] = v.GetBlockRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (v *Value) GetBlockRefCtor(id uint32) block.Ctor {
	return nil
}

// ApplySubBlock applies a sub-block change with a field id.
func (v *Value) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 4:
		if next == nil {
			v.BucketRef = nil
			return nil
		}
		sb, ok := next.(*bucket.ObjectRef)
		if !ok {
			return block.ErrUnexpectedType
		}
		v.BucketRef = sb
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (v *Value) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = v.GetBucketRef()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (v *Value) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 4:
		return func(create bool) block.SubBlock {
			n := v.GetBucketRef()
			if n == nil && create {
				n = &bucket.ObjectRef{}
				v.BucketRef = n
			}
			return n
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Value)(nil))
	_ block.SubBlock           = ((*Value)(nil))
	_ block.BlockWithRefs      = ((*Value)(nil))
	_ block.BlockWithSubBlocks = ((*Value)(nil))
	_ sbset.NamedSubBlock      = ((*Value)(nil))
)
