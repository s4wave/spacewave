package block

import "github.com/pkg/errors"

// EncodeBlockObject encodes a block's bytes with its outgoing refs.
func EncodeBlockObject(data []byte, refs []*BlockRef) ([]byte, error) {
	return (&BlockObject{Data: data, Refs: refs}).MarshalVT()
}

// DecodeBlockObject decodes an encoded BlockObject. The refs are known: an
// empty list is a leaf. The caller verifies the bytes against their ref.
func DecodeBlockObject(value []byte) (*StoredBlock, error) {
	obj := &BlockObject{}
	if err := obj.UnmarshalVT(value); err != nil {
		return nil, errors.Wrap(err, "decode block object")
	}
	return &StoredBlock{Data: obj.GetData(), Refs: obj.GetRefs(), RefsKnown: true}, nil
}
