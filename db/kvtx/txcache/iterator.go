package kvtx_txcache

import (
	"context"

	kvtx_iterator "github.com/s4wave/spacewave/db/kvtx/iterator"
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
func (i *iterOps) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	return i.t.Get(ctx, key)
}

// ScanPrefixKeys iterates over keys only with a prefix.
func (i *iterOps) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return i.t.scanPrefixUnsorted(ctx, prefix, func(key, value []byte) error {
		return cb(key)
	})
}

// _ is a type assertion
var _ kvtx_iterator.Ops = ((*iterOps)(nil))
