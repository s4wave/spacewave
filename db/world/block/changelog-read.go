package world_block

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// ChangeLogReadOptions bounds changelog traversal.
type ChangeLogReadOptions struct {
	// Limit is the maximum number of ChangeLogLL entries to return. Zero means
	// no explicit limit.
	Limit uint64
	// AfterSeqno stops traversal before entries at or below this seqno.
	AfterSeqno uint64
}

// ChangeLogEntry is one linked-list entry plus its expanded change batch.
type ChangeLogEntry struct {
	Seqno      uint64
	ChangeType WorldChangeType
	TotalSize  uint32
	Changes    []*WorldChange
}

// ReadChangeLogEntries reads recent changelog entries from a World storage accessor.
func ReadChangeLogEntries(
	ctx context.Context,
	access world.AccessWorldStateFunc,
	opts ChangeLogReadOptions,
) ([]*ChangeLogEntry, error) {
	var entries []*ChangeLogEntry
	err := access(ctx, nil, func(root *world.WorldAccess) error {
		_, rootBcs := root.BuildTransaction(nil)
		var err error
		entries, err = ReadChangeLogEntriesFromCursor(ctx, rootBcs, opts)
		return err
	})
	return entries, err
}

// ReadChangeLogEntriesFromCursor reads recent changelog entries from a cursor at the World root.
func ReadChangeLogEntriesFromCursor(
	ctx context.Context,
	rootBcs *block.Cursor,
	opts ChangeLogReadOptions,
) ([]*ChangeLogEntry, error) {
	worldRoot, err := UnmarshalWorld(ctx, rootBcs)
	if err != nil || worldRoot == nil || worldRoot.GetLastChange().GetSeqno() == 0 {
		return nil, err
	}

	entryBcs := rootBcs.FollowSubBlock(3)
	entry := worldRoot.GetLastChange()
	entries := make([]*ChangeLogEntry, 0)
	for entry != nil && entry.GetSeqno() != 0 {
		if opts.AfterSeqno != 0 && entry.GetSeqno() <= opts.AfterSeqno {
			break
		}
		changes, err := readWorldChangeBatch(ctx, entryBcs.FollowSubBlock(3), entry.GetChangeBatch())
		if err != nil {
			return nil, err
		}
		entries = append(entries, &ChangeLogEntry{
			Seqno:      entry.GetSeqno(),
			ChangeType: entry.GetChangeType(),
			TotalSize:  entry.GetChangeBatch().GetTotalSize(),
			Changes:    changes,
		})
		if opts.Limit != 0 && uint64(len(entries)) >= opts.Limit {
			break
		}
		if entry.GetPrevRef().GetEmpty() {
			break
		}
		entryBcs = entryBcs.FollowRef(2, entry.GetPrevRef())
		entry, err = UnmarshalChangeLogLL(ctx, entryBcs)
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func readWorldChangeBatch(
	ctx context.Context,
	batchBcs *block.Cursor,
	batch *WorldChangeLL,
) ([]*WorldChange, error) {
	var chunks [][]*WorldChange
	for batch != nil && !batch.IsEmpty() {
		chunks = append(chunks, slices.Clone(batch.GetChanges()))
		if batch.GetPrevRef().GetEmpty() {
			break
		}
		batchBcs = batchBcs.FollowRef(2, batch.GetPrevRef())
		var err error
		batch, err = UnmarshalWorldChangeLL(ctx, batchBcs)
		if err != nil {
			return nil, err
		}
	}

	var changes []*WorldChange
	for i := len(chunks) - 1; i >= 0; i-- {
		changes = append(changes, chunks[i]...)
	}
	return changes, nil
}
