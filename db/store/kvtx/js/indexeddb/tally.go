//go:build js

package store_kvtx_indexeddb

import (
	"context"
	"encoding/binary"

	"github.com/s4wave/spacewave/db/util/jsbuf"
)

// tallyKey is the metadata key for tracking total storage byte size.
// Uses a prefix that cannot collide with encrypted kvkey output.
var tallyKey = []byte("__hydra_storage_tally__")

// readTally reads the current tally value from the object store within a transaction.
func (t *kvtxTx) readTally(ctx context.Context) (uint64, error) {
	keyVal, err := jsbuf.CopyBytesToJs(tallyKey)
	if err != nil {
		return 0, err
	}
	val, err := t.store.Get(ctx, keyVal)
	if err != nil {
		return 0, err
	}
	if val.IsNull() || val.IsUndefined() {
		return 0, nil
	}
	data, err := jsbuf.CopyBytesToGo(val)
	if err != nil {
		return 0, err
	}
	if len(data) < 8 {
		return 0, nil
	}
	return binary.LittleEndian.Uint64(data), nil
}

// writeTally writes the tally value to the object store within a transaction.
func (t *kvtxTx) writeTally(ctx context.Context, val uint64) error {
	keyVal, err := jsbuf.CopyBytesToJs(tallyKey)
	if err != nil {
		return err
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, val)
	valVal, err := jsbuf.CopyBytesToJs(buf)
	if err != nil {
		return err
	}
	return t.store.PutKey(ctx, keyVal, valVal)
}

// updateTallyOnSet updates the tally when a key is set.
// Adds the new value length, subtracts the old value length if the key existed.
func (t *kvtxTx) updateTallyOnSet(ctx context.Context, key, value []byte) error {
	tally, err := t.readTally(ctx)
	if err != nil {
		return err
	}

	// Check if key already exists and get old value size.
	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return err
	}
	oldVal, err := t.store.Get(ctx, keyVal)
	if err != nil {
		return err
	}
	if !oldVal.IsNull() && !oldVal.IsUndefined() {
		oldData, err := jsbuf.CopyBytesToGo(oldVal)
		if err != nil {
			return err
		}
		oldLen := uint64(len(oldData))
		if tally >= oldLen {
			tally -= oldLen
		}
	}

	tally += uint64(len(value))
	return t.writeTally(ctx, tally)
}

// updateTallyOnDelete updates the tally when a key is deleted.
// Subtracts the old value length if the key existed.
func (t *kvtxTx) updateTallyOnDelete(ctx context.Context, key []byte) error {
	tally, err := t.readTally(ctx)
	if err != nil {
		return err
	}

	keyVal, err := jsbuf.CopyBytesToJs(key)
	if err != nil {
		return err
	}
	oldVal, err := t.store.Get(ctx, keyVal)
	if err != nil {
		return err
	}
	if !oldVal.IsNull() && !oldVal.IsUndefined() {
		oldData, err := jsbuf.CopyBytesToGo(oldVal)
		if err != nil {
			return err
		}
		oldLen := uint64(len(oldData))
		if tally >= oldLen {
			tally -= oldLen
		}
	}

	return t.writeTally(ctx, tally)
}
