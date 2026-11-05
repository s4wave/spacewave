package block

import "github.com/pkg/errors"

// EncodeBlockObject encodes a block's bytes with its outgoing refs, or marks
// the refs unknown when refsKnown is false.
func EncodeBlockObject(data []byte, refs []*BlockRef, refsKnown bool) ([]byte, error) {
	if !refsKnown {
		refs = nil
	}
	return (&BlockObject{Data: data, Refs: refs, RefsUnknown: !refsKnown}).MarshalVT()
}

// DecodeBlockObject decodes an encoded BlockObject. Unless the object marks
// its refs unknown, they are known and an empty list is a leaf. The caller
// verifies the bytes against their ref.
func DecodeBlockObject(value []byte) (*StoredBlock, error) {
	obj := &BlockObject{}
	if err := obj.UnmarshalVT(value); err != nil {
		return nil, errors.Wrap(err, "decode block object")
	}
	return &StoredBlock{Data: obj.GetData(), Refs: obj.GetRefs(), RefsKnown: !obj.GetRefsUnknown()}, nil
}
