package kvtx_prefixer

import (
	"github.com/aperturerobotics/hydra/kvtx"
)

// Prefixer prefixes a kvtx store.
type Prefixer struct {
	base   kvtx.Store
	prefix []byte
}

// NewPrefixer constructs a new object store prefixer.
func NewPrefixer(base kvtx.Store, prefix []byte) kvtx.Store {
	return &Prefixer{prefix: prefix, base: base}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (p *Prefixer) NewTransaction(write bool) (kvtx.Tx, error) {
	btx, err := p.base.NewTransaction(write)
	if err != nil {
		return nil, err
	}
	return newTx(btx, p.prefix), nil
}

// _ is a type assertion
var _ kvtx.Store = ((*Prefixer)(nil))
