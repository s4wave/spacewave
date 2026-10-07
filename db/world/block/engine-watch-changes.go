package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
)

// WatchChanges calls cb with each later seqno and the changes since the
// previous one, read from the World changelog. The ChangeSet is unknown when
// the changelog is disabled, no longer holds every entry since the previous
// seqno, or no longer continues from the entry it last reported. Each read
// covers every entry since the previous call, so calls coalesce while cb runs.
func (e *Engine) WatchChanges(
	ctx context.Context,
	afterSeqno uint64,
	cb func(seqno uint64, changes *world.ChangeSet) error,
) error {
	var base *ChangeLogEntry
	for {
		// Wait for the head to pass the base seqno.
		if _, err := e.WaitSeqno(ctx, afterSeqno+1); err != nil {
			return err
		}

		// Read the changes since the base from one head snapshot.
		var seqno uint64
		var head *ChangeLogEntry
		var changes *world.ChangeSet
		err := e.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
			_, rootBcs := cursor.BuildTransaction(nil)
			var err error
			seqno, head, changes, err = readChangeSet(ctx, rootBcs, afterSeqno, base)
			return err
		})
		if err != nil {
			return err
		}

		// Report the changes and advance the base.
		if err := cb(seqno, changes); err != nil {
			return err
		}
		afterSeqno, base = seqno, head
	}
}

// readChangeSet reads the head seqno, the head entry, and the changes after
// afterSeqno from the World at rootBcs. base is the entry at afterSeqno when
// known; the history must still contain it to report precise changes.
func readChangeSet(
	ctx context.Context,
	rootBcs *block.Cursor,
	afterSeqno uint64,
	base *ChangeLogEntry,
) (uint64, *ChangeLogEntry, *world.ChangeSet, error) {
	// A disabled or regressed changelog cannot describe the changes.
	root, err := UnmarshalWorld(ctx, rootBcs)
	if err != nil {
		return 0, nil, nil, err
	}
	seqno := root.GetLastChange().GetSeqno()
	if root.GetLastChangeDisable() || seqno <= afterSeqno {
		return seqno, nil, world.NewUnknownChangeSet(), nil
	}

	// Read the entries after the base, and the base entry when it is known.
	readAfter := afterSeqno
	if base != nil {
		readAfter--
	}
	entries, err := ReadChangeLogEntriesFromCursor(ctx, rootBcs, ChangeLogReadOptions{AfterSeqno: readAfter})
	if err != nil {
		return 0, nil, nil, err
	}
	if len(entries) == 0 {
		return seqno, nil, world.NewUnknownChangeSet(), nil
	}
	head := entries[0]

	// Require the history to continue from the base entry.
	if base != nil {
		last := entries[len(entries)-1]
		if !sameChangeLogEntry(base, last) {
			return seqno, head, world.NewUnknownChangeSet(), nil
		}
		entries = entries[:len(entries)-1]
	}

	// Require every entry after the base, then collect their keys and quads.
	if uint64(len(entries)) != seqno-afterSeqno {
		return seqno, head, world.NewUnknownChangeSet(), nil
	}
	changes := &world.ChangeSet{}
	for _, entry := range entries {
		if len(entry.Changes) == 0 {
			return seqno, head, world.NewUnknownChangeSet(), nil
		}
		for _, change := range entry.Changes {
			if !addWorldChange(changes, change) {
				return seqno, head, world.NewUnknownChangeSet(), nil
			}
		}
	}
	return seqno, head, changes, nil
}

// addWorldChange adds the keys or quad of change to changes.
// Returns false if the change does not name what it changed.
func addWorldChange(changes *world.ChangeSet, change *WorldChange) bool {
	switch change.GetChangeType() {
	case WorldChangeType_WorldChange_GRAPH_SET, WorldChangeType_WorldChange_GRAPH_DELETE:
		if change.GetQuad() == nil {
			return false
		}
		changes.Quads = append(changes.Quads, change.GetQuad())
	default:
		if change.GetKey() == "" {
			return false
		}
		changes.Keys = append(changes.Keys, change.GetKey())
		if newKey := change.GetNewKey(); newKey != "" {
			changes.Keys = append(changes.Keys, newKey)
		}
	}
	return true
}

// sameChangeLogEntry checks whether two entries hold the same changes.
func sameChangeLogEntry(a, b *ChangeLogEntry) bool {
	if a.Seqno != b.Seqno || a.ChangeType != b.ChangeType || len(a.Changes) != len(b.Changes) {
		return false
	}
	for i, change := range a.Changes {
		if !change.EqualVT(b.Changes[i]) {
			return false
		}
	}
	return true
}

// _ is a type assertion
var _ world.ChangeWatcher = (*Engine)(nil)
