package world

import (
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/pkg/errors"
)

// AssertObjectRev asserts that an object is at a given rev.
func AssertObjectRev(obj ObjectState, expected uint64) error {
	_, rev, err := obj.GetRootRef()
	if err == nil && rev != expected {
		err = errors.Wrapf(ErrUnexpectedRev, "expected %d got %d", expected, rev)
	}
	return err
}

// LookupRootRef gets an object and returns its root reference and rev.
//
// If not found, returns nil, 0, nil.
func LookupRootRef(eng Engine, key string) (*bucket.ObjectRef, uint64, error) {
	stx, err := eng.NewTransaction(false)
	if err != nil {
		return nil, 0, err
	}
	defer stx.Discard()

	obj, found, err := stx.GetObject(key)
	if err != nil {
		return nil, 0, err
	}

	if !found {
		return nil, 0, nil
	}
	return obj.GetRootRef()
}
