// Package hidalgo implements the high-level database abstractions for Go
// interfaces for kvtx.
package kvtx_hidalgo

import (
	"github.com/aperturerobotics/hydra/kvtx"
	kv "github.com/hidal-go/hidalgo/kv/flat"
)

// KV implements the hidalgo k/v interface with a kvtx Store.
//
// Use hidalgo/kv/flat.Upgrade if [][]byte keys are needed.
type KV struct {
	// store is the KVTx store
	store kvtx.Store
}

// NewKV constructs a new hidalgo KV wrapper.
func NewKV(store kvtx.Store) *KV {
	return &KV{store: store}
}

// Tx starts a transaction.
func (k *KV) Tx(rw bool) (kv.Tx, error) {
	tx, err := k.store.NewTransaction(rw)
	if err != nil {
		return nil, err
	}
	return NewTx(tx), nil
}

// Close closes the store.
func (k *KV) Close() error {
	// TODO
	return nil
}

// _ is a type assertion
var _ kv.KV = ((*KV)(nil))
