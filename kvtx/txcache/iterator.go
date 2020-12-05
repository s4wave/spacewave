package kvtx_txcache

import (
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// iterOps implements iterator store and returns unsorted ScanPrefixKeys.
//
// this is to ensure ScanPrefixKeys returns unsorted results (iterator sorts)
type iterOps struct {
	t *TXCache
}

// newIterOps constructs a new iterOps.
func newIterOps(t *TXCache) *iterOps {
	return &iterOps{t: t}
}

// Get returns values for a key.
func (i *iterOps) Get(key []byte) (data []byte, found bool, err error) {
	return i.t.Get(key)
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (i *iterOps) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	return i.t.scanPrefixUnsorted(prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// _ is a type assertion
var _ kvtx_iterator.Ops = ((*iterOps)(nil))
