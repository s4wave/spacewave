package git_block

import (
	"bytes"
	"io"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/pkg/errors"
)

// EncodedObjectIter iterates over the encoded object storage.
type EncodedObjectIter struct {
	r  *Store
	it kvtx.BlockIterator
}

// NewEncodedObjectIter constructs the iterator from a iavl iterator.
func NewEncodedObjectIter(r *Store, it kvtx.BlockIterator) *EncodedObjectIter {
	return &EncodedObjectIter{r: r, it: it}
}

// Next advances the iterator.
func (i *EncodedObjectIter) Next() (plumbing.EncodedObject, error) {
	if !i.it.Next() {
		return nil, io.EOF
	}
	key := i.it.Key()
	// expect byte type + hash (sha1)
	if len(key) != 21 {
		return nil, errors.Errorf("unexpected enc object tree key length %d", len(key))
	}
	keyHash := key[1:]
	encObjCs := i.it.ValueCursor()
	encObj := NewStoreEncodedObject(i.r, encObjCs)
	encObjBlk, err := encObj.unmarshalEncodedObject()
	if err != nil {
		return nil, err
	}
	if encObjBlk.GetDataHash().GetHashType() != hash.HashType_HashType_SHA1 {
		return nil, ErrHashTypeInvalid
	}
	encObjHash, err := FromHash(encObjBlk.GetDataHash())
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(encObjHash[:], keyHash[:]) {
		var keyh plumbing.Hash
		copy(keyh[:], keyHash)
		return nil, errors.Wrapf(
			ErrHashMismatch,
			"key has hash %s but got %s",
			keyh.String(),
			encObjHash.String(),
		)
	}
	return encObj, nil
}

// ForEach calls the function for each element in the iterator.
func (i *EncodedObjectIter) ForEach(cb func(plumbing.EncodedObject) error) error {
	for {
		encObj, err := i.Next()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
		if encObj == nil {
			return block.ErrUnexpectedType
		}
		if err := cb(encObj); err != nil {
			return err
		}
	}
}

// Close closes the iterator.
func (i *EncodedObjectIter) Close() {
	i.it.Close()
}

// _ is a type assertion
var _ storer.EncodedObjectIter = ((*EncodedObjectIter)(nil))
