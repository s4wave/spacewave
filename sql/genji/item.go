package kvtx_genji

import gengine "github.com/genjidb/genji/engine"

// item implements the item interface.
type item struct {
	s   *Store
	key []byte
	// value can be nil
	value *[]byte
}

// newItem constructs a new item.
func newItem(s *Store, key []byte, value *[]byte) *item {
	return &item{s: s, key: key, value: value}
}

// Key returns the key of the item.
// The key is only guaranteed to be valid until the next call to the Next method of
// the iterator.
func (i *item) Key() []byte {
	return i.key
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough, it must create a new one and return it.
func (i *item) ValueCopy(bt []byte) ([]byte, error) {
	if valuePtr := i.value; valuePtr != nil {
		return append(bt[:0], (*valuePtr)...), nil
	}

	val, err := i.s.Get(i.key)
	if err != nil {
		return nil, err
	}
	return val.ValueCopy(bt)
}

// _ is a type assertion
var _ gengine.Item = ((*item)(nil))
