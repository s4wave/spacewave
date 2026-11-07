package kvtx

import (
	"bytes"
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
)

// MoveSideGraph moves a reference graph a store kept beside a Volume into the
// Volume, so opening the Volume brings it to the current format by the
// store's recorded version, as it does a graph kept in place. root holds the
// Volume's records under volume, the graph's version 2 records under legacy
// and its later records under current, which move to the Volume's version 2
// graph prefix and its liveness prefix, where format 6 kept them. Format 7
// keeps the child lists among them in place, carries the named roots, and
// deletes the rest. A rerun after a crash resumes the move, and a root
// holding no graph costs two scans.
func MoveSideGraph(ctx context.Context, root kvtx.Store, volume, legacy, current []byte) error {
	if err := movePrefix(ctx, root, legacy, slices.Concat(volume, []byte("refs/"))); err != nil {
		return err
	}
	return movePrefix(ctx, root, current, slices.Concat(volume, livenessPrefix))
}

// movePrefix moves every record under from to the same suffix under to, as
// rekeyPrefix describes. Neither prefix may contain the other.
func movePrefix(ctx context.Context, store kvtx.Store, from, to []byte) error {
	return rekeyPrefix(ctx, store, from, from, func(key []byte) ([]byte, error) {
		return slices.Concat(to, key[len(from):]), nil
	})
}

// rekeyPrefix moves every record under prefix at or after start to the key
// rekey names for it, which must sort outside the records not yet moved. Each
// batch of up to migrateBatchBytes writes its records and deletes the
// originals in one commit, so a rerun resumes with the records not yet moved,
// and the next batch seeks past the records earlier batches moved.
func rekeyPrefix(ctx context.Context, store kvtx.Store, prefix, start []byte, rekey func(key []byte) ([]byte, error)) error {
	for start != nil {
		var err error
		if start, err = rekeyBatch(ctx, store, prefix, start, rekey); err != nil {
			return err
		}
	}
	return nil
}

// rekeyBatch moves up to migrateBatchBytes of the records under prefix at or
// after start in one commit, and returns the key to resume from, or nil when
// none remain.
func rekeyBatch(ctx context.Context, store kvtx.Store, prefix, start []byte, rekey func(key []byte) ([]byte, error)) ([]byte, error) {
	// Collect the next batch under the writer.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	moves, more, err := collectMoves(ctx, tx, prefix, start)
	if err != nil || len(moves) == 0 {
		return nil, err
	}

	// Rewrite each record under its new key and delete the original.
	for _, m := range moves {
		key, err := rekey(m.Key)
		if err != nil {
			return nil, err
		}
		if err := tx.Set(ctx, key, m.Value); err != nil {
			return nil, err
		}
		if err := tx.Delete(ctx, m.Key); err != nil {
			return nil, err
		}
	}
	if err := kvtx.CommitOrdered(ctx, tx); err != nil || !more {
		return nil, err
	}
	return keyAfter(moves[len(moves)-1].Key), nil
}

// collectMoves copies up to migrateBatchBytes of the records under prefix at
// or after start, and reports whether more remain.
func collectMoves(ctx context.Context, tx kvtx.Tx, prefix, start []byte) ([]CopyKey, bool, error) {
	// Position the sorted iterator at start.
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	if err := it.Seek(start); err != nil {
		return nil, false, err
	}

	// Copy records until the batch fills.
	var moves []CopyKey
	var size int
	for ; it.Valid() && size < migrateBatchBytes; it.Next() {
		value, err := it.ValueCopy(nil)
		if err != nil {
			return nil, false, err
		}
		key := bytes.Clone(it.Key())
		moves = append(moves, CopyKey{Key: key, Value: value})
		size += len(key) + len(value)
	}
	return moves, it.Valid(), it.Err()
}
