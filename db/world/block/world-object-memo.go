package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
)

// HasObject reports whether an object exists at key. It answers from the
// transaction-local object-existence memo without a storage read when the key
// is already known to exist, and otherwise checks the object tree, recording a
// positive result for the rest of the transaction. Positive existence is stable
// within a transaction snapshot, so read-only states memo it safely; the memo
// is invalidated when the object is deleted or renamed and reset when the
// transaction rebuilds or discards its block state.
func (t *WorldState) HasObject(ctx context.Context, key string) (bool, error) {
	if t.objectExistsKnown(key) {
		return true, nil
	}
	if t.discarded.Load() {
		return false, tx.ErrDiscarded
	}
	k := []byte(objectKeyPrefix + key)
	exists, err := t.objTree.Exists(ctx, k)
	if err != nil {
		return false, err
	}
	if exists {
		t.markObjectExists(key)
	}
	return exists, nil
}

// objectExistsKnown reports whether key is recorded in the object-existence
// memo as known to exist for the current transaction.
func (t *WorldState) objectExistsKnown(key string) bool {
	_, ok := t.objectExistsMemo[key]
	return ok
}

// markObjectExists records that an object exists at key for the rest of the
// current transaction.
func (t *WorldState) markObjectExists(key string) {
	if t.objectExistsMemo == nil {
		t.objectExistsMemo = make(map[string]struct{})
	}
	t.objectExistsMemo[key] = struct{}{}
}

// forgetObject drops key from the object-existence memo when its object is
// deleted or renamed, so a later HasObject re-checks the object tree rather than
// trusting stale transaction-local knowledge.
func (t *WorldState) forgetObject(key string) {
	if t.objectExistsMemo == nil {
		return
	}
	delete(t.objectExistsMemo, key)
}
