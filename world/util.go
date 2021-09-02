package world

import "github.com/pkg/errors"

// AssertObjectRev asserts that an object is at a given rev.
func AssertObjectRev(obj ObjectState, expected uint64) error {
	_, rev, err := obj.GetRootRef()
	if err == nil && rev != expected {
		err = errors.Wrapf(ErrUnexpectedRev, "expected %d got %d", expected, rev)
	}
	return err
}
