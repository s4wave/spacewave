package packfile

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// DecodeBlockValue decodes a pack entry: the key is the BlockKey of the block
// hash and the value is the encoded block.BlockObject of the block's bytes and
// refs. DecodeBlockValue verifies the bytes against the key. For a value that
// does not decode, or whose bytes fail their hash, DecodeBlockValue returns a
// block.CorruptError, so a reader with another source tries that source.
func DecodeBlockValue(key, value []byte) (*block.BlockRef, *block.StoredBlock, error) {
	// Resolve the packed entry key to its expected block reference.
	h, err := ParseBlockKey(key)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse block hash key")
	}
	ref := block.NewBlockRef(h)

	// Decode the packed block and verify its data against the entry key.
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		return nil, nil, &block.CorruptError{Ref: ref}
	}
	if err := block.VerifyRead(ref, stored.Data, ""); err != nil {
		return nil, nil, err
	}
	return ref, stored, nil
}
