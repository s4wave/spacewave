//go:build !tinygo

package block_store_s3

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// mergeTail writes batch together with the blocks of the newest packfile
// smaller than mergeTailBytes, when the two fit one upload batch, and replaces
// that packfile with the merged one. It reports false when no packfile
// qualifies or the merge fails before the tail is replaced, and the caller
// writes the batch alone.
//
// Invariant: the merged packfile and its entry are written, and hold every
// block the tail held, before the tail's entry and then its packfile are
// deleted, so each block of the tail stays listed in a complete packfile at
// every moment. A reader that opened the tail before it went finds it deleted
// and lists the entries again, as lookup describes. A failure before the
// delete leaves the tail in place; a failure during it leaves both packfiles,
// whose blocks repeat but are not lost. Two writers merging the same tail each
// keep all of its blocks in their own output.
//
// Merges run under compactMtx, so one store merges one tail at a time and no
// compaction or reclaim pass reads a tail being replaced.
func (s *PackStore) mergeTail(ctx context.Context, batch []*block.PutBatchEntry) (bool, error) {
	// Size the batch; a batch that fills a packfile on its own never merges.
	var batchBytes int64
	for _, entry := range batch {
		batchBytes += int64(len(entry.Data))
	}
	if batchBytes >= packPayloadBytes {
		return false, nil
	}

	// List the entries once, so a store that has only written sees the tail
	// that an earlier process left. Without them the batch is written alone.
	if err := s.loadEntries(ctx); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		s.le.WithError(err).Warn("write the batch without a tail merge: list the packfiles")
		return false, nil
	}

	// Serialize with compaction, reclaim and other merges.
	release, err := s.compactMtx.Lock(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	// Choose the newest packfile that is small enough to merge into.
	tail := s.pickTail(packPayloadBytes - batchBytes)
	if tail == nil {
		return false, nil
	}

	// Read the tail whole. A tail that is gone or unreadable is left alone.
	values, err := s.readPack(ctx, tail.GetId())
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		s.le.WithError(err).WithField("pack", tail.GetId()).Warn("write the batch beside the unreadable tail packfile")
		return false, nil
	}

	// Add the batch's blocks. A batch block that knows its refs replaces the
	// tail's copy, as a newer packfile would shadow it without a merge; one
	// that does not keeps the tail's copy and the refs it holds.
	for _, entry := range batch {
		key := string(packfile.BlockKey(entry.Ref.GetHash()))
		if _, ok := values[key]; ok && !entry.RefsKnown {
			continue
		}
		value, err := block.EncodeBlockObject(entry.Data, entry.Refs)
		if err != nil {
			return false, errors.Wrap(err, "encode block")
		}
		values[key] = value
	}

	// Write the merged packfile and list it before the tail goes.
	merged, err := s.writeValues(ctx, slices.Sorted(maps.Keys(values)), values)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		s.le.WithError(err).WithField("pack", tail.GetId()).Warn("write the batch beside the tail packfile it could not merge")
		return false, nil
	}
	s.published(merged)

	// Delete the tail. The batch is durable in the merged packfile, so a
	// failed delete only leaves the tail's blocks twice.
	if err := s.replace(ctx, nil, []string{tail.GetId()}); err != nil {
		if ctx.Err() != nil {
			return true, nil
		}
		s.le.WithError(err).WithField("pack", tail.GetId()).Warn("delete the merged tail packfile")
	}
	return true, nil
}

// pickTail returns the newest known packfile smaller than both mergeTailBytes
// and limit, or nil when the newest small packfile is not.
func (s *PackStore) pickTail(limit int64) *packfile.PackfileEntry {
	var tail *packfile.PackfileEntry
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		for _, entry := range s.entries {
			if int64(entry.GetSizeBytes()) >= s.mergeTailBytes { //nolint:gosec // a packfile is at most writer.DefaultMaxPackBytes.
				continue
			}
			if tail == nil || cmp.Or(
				entry.GetCreatedAt().AsTime().Compare(tail.GetCreatedAt().AsTime()),
				strings.Compare(entry.GetId(), tail.GetId()),
			) > 0 {
				tail = entry
			}
		}
	})
	if tail == nil || int64(tail.GetSizeBytes()) >= limit { //nolint:gosec // a packfile is at most writer.DefaultMaxPackBytes.
		return nil
	}
	return tail
}
