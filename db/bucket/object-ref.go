package bucket

import (
	jsoniter "github.com/aperturerobotics/json-iterator-lite"
	"github.com/aperturerobotics/protobuf-go-lite/json"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	transform "github.com/s4wave/spacewave/db/block/transform"
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
	return UnmarshalObjectRef(dat)
}

// UnmarshalObjectRef attempts to unmarshal an ObjectRef from bytes.
func UnmarshalObjectRef(dat []byte) (*ObjectRef, error) {
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
func (o *ObjectRef) IsNil() bool {
	return o == nil
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
	if !o.GetRootRef().GetEmpty() {
		if err := o.GetRootRef().Validate(false); err != nil {
			return err
		}
	}
	if !o.GetTransformConfRef().GetEmpty() {
		if err := o.GetTransformConfRef().Validate(false); err != nil {
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
//
// NOTE: ignores if BucketId is empty.
func (o *ObjectRef) GetEmpty() bool {
	if o == nil {
		return true
	}
	return o.GetRootRef().GetEmpty() &&
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
	return o.MarshalB58()
}

// MarshalB58 marshals the reference to a base58 string form.
func (o *ObjectRef) MarshalB58() string {
	if o == nil {
		return ""
	}
	dat, err := o.MarshalVT()
	if err != nil {
		return ""
	}
	return b58.Encode(dat)
}

// UnmarshalB58 unmarshals the reference from base58 string form.
func (o *ObjectRef) UnmarshalB58(ref string) error {
	o.Reset()
	if ref == "" {
		return nil
	}

	dat, err := b58.Decode(ref)
	if err != nil {
		return err
	}
	return o.UnmarshalVT(dat)
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
		return transform.NewTransformConfigBlock
	}
	return nil
}

// MarshalProtoJSON marshals the ObjectRef message to JSON.
func (o *ObjectRef) MarshalProtoJSON(s *json.MarshalState) {
	if o == nil {
		s.WriteNil()
		return
	}
	s.WriteObjectStart()
	var wroteField bool
	if o.RootRef != nil || s.HasField("rootRef") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("rootRef")
		o.RootRef.MarshalProtoJSON(s.WithField("rootRef"))
	}
	if o.BucketId != "" || s.HasField("bucketId") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("bucketId")
		s.WriteString(o.BucketId)
	}
	if o.TransformConfRef != nil || s.HasField("transformConfRef") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("transformConfRef")
		o.TransformConfRef.MarshalProtoJSON(s.WithField("transformConfRef"))
	}
	if o.TransformConf != nil || s.HasField("transformConf") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("transformConf")
		o.TransformConf.MarshalProtoJSON(s.WithField("transformConf"))
	}
	s.WriteObjectEnd()
}

// MarshalJSON marshals the ObjectRef to JSON.
func (o *ObjectRef) MarshalJSON() ([]byte, error) {
	return json.DefaultMarshalerConfig.Marshal(o)
}

// UnmarshalProtoJSON unmarshals the ObjectRef message from JSON.
func (o *ObjectRef) UnmarshalProtoJSON(s *json.UnmarshalState) {
	if s.ReadNil() {
		return
	}
	if s.WhatIsNext() == jsoniter.StringValue {
		if err := o.UnmarshalB58(s.ReadString()); err != nil {
			s.SetError(err)
		}
		return
	}
	s.ReadObject(func(key string) {
		switch key {
		default:
			s.Skip() // ignore unknown field
		case "root_ref", "rootRef":
			if s.ReadNil() {
				o.RootRef = nil
				return
			}
			o.RootRef = &block.BlockRef{}
			o.RootRef.UnmarshalProtoJSON(s.WithField("root_ref", true))
		case "bucket_id", "bucketId":
			s.AddField("bucket_id")
			o.BucketId = s.ReadString()
		case "transform_conf_ref", "transformConfRef":
			if s.ReadNil() {
				o.TransformConfRef = nil
				return
			}
			o.TransformConfRef = &block.BlockRef{}
			o.TransformConfRef.UnmarshalProtoJSON(s.WithField("transform_conf_ref", true))
		case "transform_conf", "transformConf":
			if s.ReadNil() {
				o.TransformConf = nil
				return
			}
			o.TransformConf = &transform.Config{}
			o.TransformConf.UnmarshalProtoJSON(s.WithField("transform_conf", true))
		}
	})
}

// UnmarshalJSON unmarshals the ObjectRef from JSON.
func (o *ObjectRef) UnmarshalJSON(b []byte) error {
	return json.DefaultUnmarshalerConfig.Unmarshal(b, o)
}

// _ is a type assertion
var (
	_ block.Block = ((*ObjectRef)(nil))
	// note: only refs with zero-len bucket id are returned
	_ block.BlockWithRefs = ((*ObjectRef)(nil))

	_ json.Marshaler   = ((*ObjectRef)(nil))
	_ json.Unmarshaler = ((*ObjectRef)(nil))
)
