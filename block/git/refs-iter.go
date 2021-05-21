package git

import (
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/pkg/errors"
)

// ReferenceIter iterates over the reference storage.
type ReferenceIter struct {
	r  *Store
	it kvtx.BlockIterator
}

// NewReferenceIter constructs the iterator from a iavl iterator.
func NewReferenceIter(r *Store, it kvtx.BlockIterator) *ReferenceIter {
	return &ReferenceIter{r: r, it: it}
}

// Next advances the iterator.
func (i *ReferenceIter) Next() (*plumbing.Reference, error) {
	if !i.it.Next() {
		return nil, io.EOF
	}
	key := i.it.Key()
	// expect 1 byte + name
	if len(key) < 2 {
		return nil, errors.Errorf("unexpected ref key length %d", len(key))
	}
	refName := string(key[1:])
	valCursor := i.it.ValueCursor()
	blkRef, err := byteslice.ByteSliceToRef(valCursor)
	if err != nil {
		return nil, err
	}
	encObjCs := valCursor.FollowRef(1, blkRef)
	refObjBlk, err := encObjCs.Unmarshal(NewReferenceBlock)
	if err != nil {
		return nil, err
	}
	ref, ok := refObjBlk.(*Reference)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	if ref.GetName() != refName {
		return nil, ErrReferenceNameInvalid
	}
	return ref.ToReference()
}

// ForEach calls the function for each element in the iterator.
func (i *ReferenceIter) ForEach(cb func(*plumbing.Reference) error) error {
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
func (i *ReferenceIter) Close() {
	i.it.Close()
}

// _ is a type assertion
var _ storer.ReferenceIter = ((*ReferenceIter)(nil))
