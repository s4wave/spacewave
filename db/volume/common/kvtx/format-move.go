package kvtx

import (
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

// movePrefix moves every record under from to the same suffix under to. Each
// batch writes its records and deletes the originals in one commit, so a
// rerun resumes with the records not yet moved, and seeks past the records
// earlier batches moved. Neither prefix may contain the other.
func movePrefix(ctx context.Context, store kvtx.Store, from, to []byte) error {
	for start := from; start != nil; {
		var err error
		if start, err = moveBatch(ctx, store, from, to, start); err != nil {
			return err
		}
	}
	return nil
}

// moveBatch moves up to migrateBatchBytes of the records under from at or
// after start in one commit, and returns the key to resume from, or nil when
// none remain.
func moveBatch(ctx context.Context, store kvtx.Store, from, to, start []byte) ([]byte, error) {
	// Collect the next batch under the writer.
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	moves, more, err := collectMoves(ctx, tx, from, start)
	if err != nil || len(moves) == 0 {
		return nil, err
	}

	// Rewrite each record under to and delete the original.
	for _, m := range moves {
		if err := tx.Set(ctx, slices.Concat(to, m.key[len(from):]), m.value); err != nil {
			return nil, err
		}
		if err := tx.Delete(ctx, m.key); err != nil {
			return nil, err
		}
	}
	if err := kvtx.CommitOrdered(ctx, tx); err != nil || !more {
		return nil, err
	}
	return keyAfter(moves[len(moves)-1].key), nil
}
