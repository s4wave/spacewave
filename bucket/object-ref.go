package bucket

import (
	"encoding/json"
	"strconv"

	"github.com/aperturerobotics/hydra/block"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	jsonpb "google.golang.org/protobuf/encoding/protojson"
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
	if err := o.UnmarshalVT(dat); err != nil {
		return nil, err
	}
	return o, nil
}

// UnmarshalObjectRefJSON attempts to unmarshal an ObjectRef from JSON.
func UnmarshalObjectRefJSON(data []byte) (*ObjectRef, error) {
	ref := &ObjectRef{}
	if err := ref.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return ref, nil
}

// MarshalObjectRefJSON marshals an ObjectRef to JSON.
//
// Returns "{}" if the ref is nil.
func MarshalObjectRefJSON(ref *ObjectRef) ([]byte, error) {
	return ref.MarshalJSON()
}

// IsNil returns if the object is nil.
func (r *ObjectRef) IsNil() bool {
	return r == nil
}

// ParseFromB58 parses the object ref from a base58 string.
func (o *ObjectRef) ParseFromB58(ref string) error {
	dat, err := b58.Decode(ref)
	if err != nil {
		return err
	}
	return o.UnmarshalVT(dat)
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
func (o *ObjectRef) GetEmpty() bool {
	if o == nil {
		return true
	}
	return o.GetRootRef().GetEmpty() &&
		o.GetBucketId() == "" &&
		o.GetTransformConfRef().GetEmpty() &&
		o.GetTransformConf().GetEmpty()
}

// EqualsRef checks if the ref is equal to another ref.
func (o *ObjectRef) EqualsRef(ot *ObjectRef) bool {
	return o.checkEqual(ot, false)
}

// EqualsRefIgnoreRootRef checks if two refs are equal except for RootRef.
func (o *ObjectRef) EqualsRefIgnoreRootRef(ot *ObjectRef) bool {
	return o.checkEqual(ot, true)
}

// checkEqual checks equality with another ref.
func (o *ObjectRef) checkEqual(ot *ObjectRef, ignoreRootRef bool) bool {
	if o == nil && ot == nil {
		return true
	}

	switch {
	case o.GetEmpty() != ot.GetEmpty():
	case !o.GetRootRef().EqualsRef(ot.GetRootRef()) && !ignoreRootRef:
	case (o.GetTransformConf() == nil) != (ot.GetTransformConf() == nil):
	case !o.GetTransformConf().EqualVT(ot.GetTransformConf()):
	case o.GetTransformConfRef().GetEmpty() != ot.GetTransformConfRef().GetEmpty():
	case !o.GetTransformConfRef().EqualsRef(ot.GetTransformConfRef()):
	case o.GetBucketId() != ot.GetBucketId():
	default:
		return true
	}

	return false
}

// Clone makes a copy of the ref.
func (o *ObjectRef) Clone() *ObjectRef {
	if o == nil {
		return nil
	}
	nref := &ObjectRef{}
	nref.CopyFrom(o)
	return nref
}

// CopyFrom copies the contents of another ObjectRef.
func (o *ObjectRef) CopyFrom(ot *ObjectRef) {
	if ot == nil {
		return
	}
	if o == nil {
		ot.Reset()
		return
	}

	o.BucketId = ot.GetBucketId()
	o.RootRef = ot.GetRootRef().Clone()
	o.TransformConf = ot.GetTransformConf().Clone()
	o.TransformConfRef = ot.GetTransformConfRef().Clone()
}

// MarshalString marshals the reference to a string form.
func (o *ObjectRef) MarshalString() string {
	if o == nil {
		return ""
	}
	dat, err := o.MarshalVT()
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *ObjectRef) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *ObjectRef) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (o *ObjectRef) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 1:
		o.RootRef = ptr
	case 3:
		o.TransformConfRef = ptr
	}
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (o *ObjectRef) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	m := make(map[uint32]*block.BlockRef, 2)
	// ignore root-ref if bucket id is not empty
	if len(o.GetBucketId()) == 0 {
		m[1] = o.GetRootRef()
	}
	m[3] = o.GetTransformConfRef()
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (o *ObjectRef) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 1:
		// unknown
	case 3:
		return block_transform.NewTransformConfigBlock
	}
	return nil
}

// MarshalJSON marshals the reference to a JSON string.
// Returns empty quotes if the ref is nil.
func (o *ObjectRef) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(o.MarshalString())), nil
}

// UnmarshalFastJSON unmarshals the fast json container.
// If the val or object ref are nil, does nothing.
func (o *ObjectRef) UnmarshalFastJSON(val *fastjson.Value) error {
	if val == nil || o == nil {
		return nil
	}
	switch val.Type() {
	case fastjson.TypeString:
		return o.ParseFromB58(string(val.GetStringBytes()))
	case fastjson.TypeObject:
	default:
		return errors.Errorf("unexpected json type for object ref: %v", val.Type().String())
	}

	// rootRef:
	if rootRefVal := val.Get("rootRef"); rootRefVal != nil {
		rootRef := o.RootRef
		if rootRef == nil {
			rootRef = &block.BlockRef{}
			o.RootRef = rootRef
		}
		if err := rootRef.UnmarshalFastJSON(rootRefVal); err != nil {
			return errors.Wrap(err, "rootRef")
		}
	}

	// bucketId
	if bucketIDVal := val.Get("bucketId"); bucketIDVal != nil {
		bucketIdBytes, err := bucketIDVal.StringBytes()
		if err != nil {
			return errors.Wrap(err, "bucketId")
		}
		o.BucketId = string(bucketIdBytes)
	}

	// transformConfRef
	if transformConfRefVal := val.Get("transformConfRef"); transformConfRefVal != nil {
		tconfRef := o.TransformConfRef
		if tconfRef == nil {
			tconfRef = &block.BlockRef{}
			o.TransformConfRef = tconfRef
		}
		if err := tconfRef.UnmarshalFastJSON(transformConfRefVal); err != nil {
			return errors.Wrap(err, "transformConfRef")
		}
	}

	// transformConf is the inline transform configuration.
	if transformConfVal := val.Get("transformConf"); transformConfVal != nil {
		tconf := o.TransformConf
		if tconf == nil {
			tconf = &block_transform.Config{}
			o.TransformConf = tconf
		}
		/*
			if err := tconf.UnmarshalFastJSON(transformConfVal); err != nil {
				return errors.Wrap(err, "transformConf")
			}
		*/
		if err := jsonpb.Unmarshal(transformConfVal.MarshalTo(nil), tconf); err != nil {
			return errors.Wrap(err, "transformConf")
		}
	}

	return nil
}

// UnmarshalJSON unmarshals the reference from a JSON string.
// Accepts a JSON object or a JSON string (b58 encoded).
func (o *ObjectRef) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || o == nil {
		return nil
	}
	val, err := fastjson.ParseBytes(data)
	if err != nil {
		return err
	}
	return o.UnmarshalFastJSON(val)
}

// _ is a type assertion
var (
	_ block.Block = ((*ObjectRef)(nil))
	// note: only refs with zero-len bucket id are returned
	_ block.BlockWithRefs = ((*ObjectRef)(nil))

	_ json.Marshaler   = ((*ObjectRef)(nil))
	_ json.Unmarshaler = ((*ObjectRef)(nil))
)
