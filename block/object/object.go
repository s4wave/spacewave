package object

import (
	"github.com/golang/protobuf/proto"
	b58 "github.com/mr-tron/base58/base58"
)

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
