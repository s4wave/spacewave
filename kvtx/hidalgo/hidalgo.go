// Package hidalgo implements the high-level database abstractions for Go
// interfaces for kvtx.
package kvtx_hidalgo

import (
	"context"

	kv "github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/hydra/kvtx"
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
func (k *KV) Tx(ctx context.Context, rw bool) (kv.Tx, error) {
	tx, err := k.store.NewTransaction(ctx, rw)
	if err != nil {
		return nil, err
	}
	return NewTx(tx), nil
}

// View creates a read transaction that will be discarded when fn returns.
func (k *KV) View(ctx context.Context, fn func(tx kv.Tx) error) error {
	return kv.View(ctx, k, fn)
}

func (k *KV) Update(ctx context.Context, fn func(tx kv.Tx) error) error {
	return kv.Update(ctx, k, fn)
}

// Close closes the store.
// NOTE: we return nil here and do nothing!
// We may re-use the same kvtx.Store for multiple KV handles.
func (k *KV) Close() error {
	return nil
}

// _ is a type assertion
var _ kv.KV = ((*KV)(nil))
