package hashmap

import (
	"sync"

	"github.com/aperturerobotics/hydra/kvtx"
)

// HashmapKvtx implements kvtx store on top of a hash map.
//
// Note: some of the kvtx conventions might not be followed.
type HashmapKvtx struct {
	m    Hashmap
	rmtx sync.RWMutex
}

// NewHashmapKvtx constructs a new Kvtx store from a hashmap.
func NewHashmapKvtx(m Hashmap) kvtx.Store {
	return &HashmapKvtx{m: m}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (m *HashmapKvtx) NewTransaction(write bool) (kvtx.Tx, error) {
	return NewHashmapKvtxTx(m, write)
}

// _ is a type assertion
var _ kvtx.Store = ((*HashmapKvtx)(nil))
