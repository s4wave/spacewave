package store_kvtx_inmem

import "bytes"

// valTypeLess implements the less function for valType comparison
// may be called with nil if we pass nil for the pivot
func valTypeLess(a, b *valType) bool {
	var aKey, bKey []byte
	if a != nil {
		aKey = a.key
	}
	if b != nil {
		bKey = b.key
	}
	return bytes.Compare(aKey, bKey) < 0
}

// valType is the type used for values
type valType struct {
	key []byte
	val []byte
}

// Less implements btree.Item interface
func (v *valType) Less(than *valType) bool {
	return valTypeLess(v, than)
}
