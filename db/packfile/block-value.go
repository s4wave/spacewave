package packfile

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// DecodeBlockValue decodes a pack entry: the key is the base58 block hash and
// the value is the encoded block.BlockObject of the block's bytes and refs.
// It verifies the bytes against the key.
func DecodeBlockValue(key, value []byte) (*block.BlockRef, *block.StoredBlock, error) {
	h := &hash.Hash{}
	if err := h.ParseFromB58(string(key)); err != nil {
		return nil, nil, errors.Wrap(err, "parse block hash key")
	}
	ref := block.NewBlockRef(h)
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		return nil, nil, errors.Wrap(err, string(key))
	}
	if err := ref.VerifyData(stored.Data, true); err != nil {
		return nil, nil, err
	}
	return ref, stored, nil
}
