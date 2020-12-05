package kvtx_prefixer

import (
	"bytes"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Iterator iterates over the store with a given prefix.
type Iterator struct {
	t      *tx
	it     kvtx.Iterator
	prefix []byte
}

// NewIterator constructs a new iterator.
func NewIterator(t *tx, prefix []byte, sort, rev bool) *Iterator {
	it := t.lower.Iterate(bytes.Join([][]byte{t.prefix, prefix}, nil), sort, rev)
	return &Iterator{t: t, it: it, prefix: prefix}
}

// Err returns any error that has closed the iterator.
// May return context.Canceled if closed.
func (i *Iterator) Err() error {
	return i.it.Err()
}

// Valid returns if the iterator points to a valid entry.
//
// If err is set, returns false.
func (i *Iterator) Valid() bool {
	return i.it.Valid()
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	k := i.it.Key()
	plen := len(i.prefix) + len(i.t.prefix)
	if len(k) <= plen {
		return nil
	}
	kn := make([]byte, len(k)-plen)
	copy(kn, k[plen:])
	return kn
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (i *Iterator) Value() []byte {
	return i.it.Value()
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	return i.it.ValueCopy(bt)
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	return i.it.Next()
}

// Seek moves the iterator to the selected key.
func (i *Iterator) Seek(k []byte) {
	if len(k) == 0 {
		i.it.Seek(nil)
	} else {
		i.it.Seek(bytes.Join([][]byte{
			i.t.prefix,
			i.prefix,
			k,
		}, nil))
	}
}

// Close closes the iterator.
func (i *Iterator) Close() {
	i.it.Close()
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
