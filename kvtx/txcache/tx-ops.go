package kvtx_txcache

import (
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
)

// Get returns values for a key.
func (t *Tx) Get(key []byte) (data []byte, found bool, err error) {
	tc := t.tc
	if tc == nil {
		return nil, false, kvtx.ErrDiscarded
	}
	return tc.Get(key)
}

// Set sets the value of a key.
// This will not be committed until Commit is called.
func (t *Tx) Set(key, value []byte, ttl time.Duration) error {
	tc := t.tc
	if tc == nil {
		return kvtx.ErrDiscarded
	}
	return tc.Set(key, value, ttl)
}

// Delete deletes a key.
// This will not be committed until Commit is called.
// Not found should not return an error.
func (t *Tx) Delete(key []byte) error {
	tc := t.tc
	if tc == nil {
		return kvtx.ErrDiscarded
	}
	return tc.Delete(key)
}

// ScanPrefix iterates over keys with a prefix.
//
// Note: neither key nor value should be retained outside cb() without
// copying.
//
// Note: the ordering of the scan is not necessarily sorted.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, value []byte) error) error {
	tc := t.tc
	if tc == nil {
		return kvtx.ErrDiscarded
	}
	return tc.ScanPrefix(prefix, cb)
}

// Exists checks if a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	tc := t.tc
	if tc == nil {
		return false, kvtx.ErrDiscarded
	}
	return tc.Exists(key)
}

// _ is a type assertion
var _ kvtx.TxOps = ((*Tx)(nil))
