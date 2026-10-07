package world

import "context"

// ChangeWatcher is an Engine that reports what changed between seqnos.
type ChangeWatcher interface {
	// WatchChanges calls cb with each later seqno and the changes since the
	// previous one, starting after afterSeqno. Calls coalesce while cb runs.
	// Returns when ctx is canceled or cb returns an error.
	WatchChanges(ctx context.Context, afterSeqno uint64, cb func(seqno uint64, changes *ChangeSet) error) error
}

// WatchChanges calls cb with each seqno of engine after afterSeqno and the
// changes since the previous call. An engine that is not a ChangeWatcher
// reports every advance as an unknown ChangeSet.
// Returns when ctx is canceled or cb returns an error.
func WatchChanges(
	ctx context.Context,
	engine Engine,
	afterSeqno uint64,
	cb func(seqno uint64, changes *ChangeSet) error,
) error {
	// Prefer the engine's own change feed.
	if watcher, ok := engine.(ChangeWatcher); ok {
		return watcher.WatchChanges(ctx, afterSeqno, cb)
	}

	// Report each seqno advance as unknown.
	for {
		seqno, err := engine.WaitSeqno(ctx, afterSeqno+1)
		if err != nil {
			return err
		}
		if err := cb(seqno, NewUnknownChangeSet()); err != nil {
			return err
		}
		afterSeqno = seqno
	}
}
