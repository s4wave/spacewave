package git

import (
	"bytes"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage"
)

// SetReference sets the reference in the block graph.
func (r *Store) SetReference(ref *plumbing.Reference) error {
	nref, err := NewReference(ref)
	if err == nil {
		err = nref.Validate()
	}
	if err != nil {
		return err
	}

	key, err := r.buildRefKey(nref.GetName())
	if err != nil {
		return err
	}

	refTree := r.refTree
	_, nodCs, err := refTree.SetCursorAsRef(key, nil)
	if err != nil {
		return err
	}
	refCs := nodCs.FollowRef(1, nil)
	refCs.SetBlock(nref, true)
	return nil
}

// Reference returns the reference by name.
func (r *Store) Reference(ref plumbing.ReferenceName) (*plumbing.Reference, error) {
	key, err := r.buildRefKey(string(ref))
	if err != nil {
		return nil, err
	}

	refBlk, _, err := r.lookupReference(key)
	if err != nil {
		return nil, err
	}
	return refBlk.ToReference()
}

// CheckAndSetReference sets the reference `new`, but if `old` is not `nil`, it
// first checks that the current stored value for `old.Name()` matches the given
// reference value in `old`. If not, it returns an error and doesn't update
// `new`.
func (r *Store) CheckAndSetReference(new, old *plumbing.Reference) error {
	if new == nil || len(new.Name()) == 0 {
		return ErrReferenceNameEmpty
	}
	if old != nil {
		oldRef, err := r.Reference(old.Name())
		if err != nil {
			return err
		}
		if oldRef != nil {
			oldHash := oldRef.Hash()
			expectedHash := old.Hash()
			if bytes.Compare(oldHash[:], expectedHash[:]) != 0 {
				return storage.ErrReferenceHasChanged
			}
		}
	}
	return r.SetReference(new)
}

// IterReferences iterates over references.
func (r *Store) IterReferences() (storer.ReferenceIter, error) {
	prefix := []byte{0x0}
	treeTx := r.refTree
	ktxIterator := treeTx.IterateIavl(prefix, false, false)
	return NewReferenceIter(r, ktxIterator), nil
}

// RemoveReference removes a reference from the storage.
func (r *Store) RemoveReference(ref plumbing.ReferenceName) error {
	key, err := r.buildRefKey(string(ref))
	if err != nil {
		return err
	}
	return r.refTree.Delete(key)
}

// CountLooseRefs counts refs without any parent ref.
func (r *Store) CountLooseRefs() (int, error) {
	return int(r.refTree.Size()), nil
}

// PackRefs packs references.
func (r *Store) PackRefs() error {
	// no-op.
	return nil
}

// buildRefKey builds the key for a reference.
func (r *Store) buildRefKey(refName string) ([]byte, error) {
	if len(refName) == 0 {
		return nil, ErrReferenceNameEmpty
	}
	// prefix with 0x0
	return append([]byte{byte(0x0)}, []byte(refName)...), nil
}

// lookupReference tries to build the Reference from a key.
func (r *Store) lookupReference(key []byte) (*Reference, *block.Cursor, error) {
	refTree := r.refTree
	_, nodCs, err := refTree.GetWithCursor(key)
	if err != nil {
		return nil, nil, err
	}
	if nodCs == nil {
		return nil, nil, plumbing.ErrReferenceNotFound
	}

	nodRef, err := byteslice.ByteSliceToRef(nodCs)
	if err != nil {
		return nil, nil, err
	}
	encObjCs := nodCs.FollowRef(1, nodRef)
	encObji, err := encObjCs.Unmarshal(NewReferenceBlock)
	if err != nil {
		return nil, nil, err
	}
	encObjBlk, ok := encObji.(*Reference)
	if !ok {
		return nil, nil, block.ErrUnexpectedType
	}
	return encObjBlk, encObjCs, nil
}

// _ is a type assertion
var (
	// ReferenceStorer stores Store refs (tags, branches, ...)
	_ storer.ReferenceStorer = (*Store)(nil)
	_ storer.DeltaObjectStorer
)
