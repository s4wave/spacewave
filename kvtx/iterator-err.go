package kvtx

import "context"

// errIterator returns an error instead of an iterator.
type errIterator struct {
	err error
}

// NewErrIterator returns an iterator that starts with an error.
func NewErrIterator(err error) Iterator {
	if err == nil {
		err = context.Canceled
	}
	return &errIterator{err: err}
}

// Err returns any error that has closed the iterator.
// May return context.Canceled if closed.
func (e *errIterator) Err() error {
	return e.err
}

// Valid returns if the iterator points to a valid entry.
func (e *errIterator) Valid() bool {
	return false
}

// Key returns the current entry key, or nil if not valid.
func (e *errIterator) Key() []byte {
	return nil
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (e *errIterator) Value() []byte {
	return nil
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (e *errIterator) ValueCopy([]byte) ([]byte, error) {
	return nil, e.err
}

// Next advances to the next entry and returns Valid.
func (e *errIterator) Next() bool {
	return false
}

// Seek moves the iterator to the selected key, or the next key after the key.
// Pass nil to seek to the beginning (or end if reversed).
func (e *errIterator) Seek(k []byte) {
	return
}

// Close closes the iterator.
func (e *errIterator) Close() {
	return
}

// _ is a type assertion
var _ Iterator = (*errIterator)(nil)
