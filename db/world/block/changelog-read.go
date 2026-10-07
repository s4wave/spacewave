package world_block

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
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

// ReadChangeLogEntries reads recent changelog entries from a World storage accessor.
func ReadChangeLogEntries(
	ctx context.Context,
	access world.AccessWorldStateFunc,
	opts ChangeLogReadOptions,
) ([]*ChangeLogEntry, error) {
	var entries []*ChangeLogEntry
	err := access(ctx, nil, func(root *bucket_lookup.Cursor) error {
		_, rootBcs := root.BuildTransaction(nil)
		var err error
		entries, err = ReadChangeLogEntriesFromCursor(ctx, rootBcs, opts)
		return err
	})
	return entries, err
}

// ReadChangeLogEntriesFromCursor reads recent changelog entries from a cursor
// at the World root, newest first. It walks the current segment, then the
// previous one, and stops at the limit, the cursor, or the oldest retained entry.
func ReadChangeLogEntriesFromCursor(
	ctx context.Context,
	rootBcs *block.Cursor,
	opts ChangeLogReadOptions,
) ([]*ChangeLogEntry, error) {
	// Unmarshal the World root and stop when it has no changelog.
	worldRoot, err := UnmarshalWorld(ctx, rootBcs)
	if err != nil || worldRoot == nil || worldRoot.GetLastChange().GetSeqno() == 0 {
		return nil, err
	}

	// Walk changelog nodes and collect entries until the limit or cursor.
	entryBcs := rootBcs.FollowSubBlock(3)
	entry := worldRoot.GetLastChange()
	prevSegment := worldRoot.GetPrevChanges()
	entries := make([]*ChangeLogEntry, 0)
	for entry.GetSeqno() > opts.AfterSeqno {
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

		// Step to the previous entry, crossing into the previous segment once.
		switch {
		case !entry.GetPrevRef().GetEmpty():
			entryBcs = entryBcs.FollowRef(2, entry.GetPrevRef())
		case !prevSegment.GetEmpty():
			entryBcs = rootBcs.FollowRef(7, prevSegment)
			prevSegment = nil
		default:
			return entries, nil
		}
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
	// Walk the change-batch linked list into chunks.
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

	// Flatten the chunks from oldest to newest.
	var changes []*WorldChange
	for _, chunk := range slices.Backward(chunks) {
		changes = append(changes, chunk...)
	}
	return changes, nil
}
