//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

const (
	// compactFanout is the number of packfiles of one tier a merge combines.
	compactFanout = 8
	// compactTiers is the number of tiers that merge. Heavier packfiles are
	// final.
	compactTiers = 3
	// compactMaxBytes is the packfile size that weighs 1, or the store's
	// MaxPackBytes when that is smaller.
	compactMaxBytes = 16 << 20
	// compactMaxBlocks is the packfile block count that weighs 1.
	compactMaxBlocks = writer.DefaultMaxBlocksPerPack
)

// Compact loads every known packfile entry and merges small packfiles until
// no tier holds compactFanout of them, as compact describes. The background
// routine merges only after this store writes, so a store reopened over the
// small packfiles an earlier process left merges them only through Compact.
func (s *PackStore) Compact(ctx context.Context) error {
	if err := s.loadAll(ctx); err != nil {
		return err
	}
	return s.compact(ctx)
}

// runCompaction runs compact after each write until ctx ends.
func (s *PackStore) runCompaction(ctx context.Context) error {
	var compacted uint64
	for {
		var writes uint64
		var wait <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			writes, wait = s.writes, getWaitCh()
		})
		if writes == compacted {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-wait:
				continue
			}
		}
		if err := s.compact(ctx); err != nil {
			return err
		}
		compacted = writes
	}
}

// compact merges small packfiles until no tier holds compactFanout of them.
//
// A packfile's weight is the larger of its share of the store's compaction
// unit, compactMaxBytes or MaxPackBytes when that is smaller, and of
// compactMaxBlocks. Weights below 1/512, 1/64, and 1/8 form tiers 0 to 2. The
// rest are final. The compactFanout packfiles of one tier merge into a
// packfile of weight below 1, so each block is rewritten at most compactTiers
// times and at most compactFanout-1 packfiles wait in each tier.
//
// The pass merges the packfiles this store wrote or last listed, without
// listing the bucket, so each writer keeps at most compactFanout-1 packfiles
// waiting per tier. A merge deletes only inputs whose every block its output
// holds, so concurrent passes on several devices lose no block. A merge that
// finds an input gone lists the entries and chooses again.
func (s *PackStore) compact(ctx context.Context) error {
	// A store that keeps its packs merges none.
	if s.keepPacks {
		return nil
	}

	// Acquire the compaction mutex so only one merge runs at a time.
	release, err := s.compactMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Merge packfiles until no tier holds compactFanout of them.
	for {
		inputs := s.pickMerge()
		if len(inputs) == 0 {
			return nil
		}
		err := s.merge(ctx, inputs)
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}

		// Another writer merged an input first: list and choose again.
		if err := s.listEntries(ctx, s.listings.Load()); err != nil {
			return err
		}
		if s.knowsAll(inputs) {
			return errors.Wrap(err, "listed packfile is missing")
		}
	}
}

// pickMerge returns the compactFanout oldest packfiles of the lowest tier that
// holds that many, or nil when no tier does.
func (s *PackStore) pickMerge() []*packfile.PackfileEntry {
	var tiers [compactTiers][]*packfile.PackfileEntry
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		for _, entry := range s.entries {
			if tier := s.packTier(entry); tier >= 0 {
				tiers[tier] = append(tiers[tier], entry)
			}
		}
	})
	for _, entries := range tiers {
		if len(entries) >= compactFanout {
			slices.SortFunc(entries, func(a, b *packfile.PackfileEntry) int {
				return cmp.Or(
					a.GetCreatedAt().AsTime().Compare(b.GetCreatedAt().AsTime()),
					strings.Compare(a.GetId(), b.GetId()),
				)
			})
			return entries[:compactFanout]
		}
	}
	return nil
}

// knowsAll reports whether every entry is still known.
func (s *PackStore) knowsAll(entries []*packfile.PackfileEntry) bool {
	known := true
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		for _, entry := range entries {
			known = known && s.entries[entry.GetId()] != nil
		}
	})
	return known
}

// merge writes the blocks of inputs as one packfile, yielded in key order and
// laid out as writePack chooses, and replaces the inputs with it. Returns
// ErrNotFound when an input is gone.
//
// Invariant: the merged packfile holds exactly the blocks its inputs hold, so
// the layout changes where a block sits and never whether it is kept. The
// merged packfile is listed before the inputs' entries and then their
// packfiles are deleted.
// Copies of one key have identical block bytes; keep the copy with the strongest
// refs source, so a later copy cannot discard refs an earlier one recorded.
func (s *PackStore) merge(ctx context.Context, inputs []*packfile.PackfileEntry) error {
	// Read the inputs concurrently.
	packs := make([]map[string][]byte, len(inputs))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchConcurrency)
	for i, input := range inputs {
		eg.Go(func() error {
			var err error
			packs[i], err = s.readPack(egCtx, input.GetId())
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// Write the union of their keys, choosing each copy while it is decoded.
	keys := make(map[string]struct{})
	for _, pack := range packs {
		for key := range pack {
			keys[key] = struct{}{}
		}
	}
	merged, err := s.writeValues(ctx, slices.Sorted(maps.Keys(keys)), packs, nil)
	if err != nil {
		return errors.Wrap(err, "write merged packfile")
	}
	if merged == nil {
		return errors.New("merged packfiles hold no blocks")
	}

	// Replace the inputs with the merged packfile.
	ids := make([]string, len(inputs))
	for i, input := range inputs {
		ids[i] = input.GetId()
	}
	return s.replace(ctx, merged, ids)
}

// writeValues writes the block values of keys, in order, as one packfile. For
// duplicate keys it keeps the strongest refs source, preferring later copies
// on a tie, with the decoded batch last. Each encoded candidate is decoded once.
// Returns nil when keys is empty.
func (s *PackStore) writeValues(ctx context.Context, keys []string, values []map[string][]byte, batch map[string]*block.StoredBlock) (*packfile.PackfileEntry, error) {
	i := 0
	return s.writePack(ctx, func() (*hash.Hash, *block.StoredBlock, error) {
		// Take the next key, or end the packfile after the last.
		if i == len(keys) {
			return nil, nil, nil
		}
		key := keys[i]
		i++

		// Decode each copy once and keep its most informative refs source.
		h, err := packfile.ParseBlockKey([]byte(key))
		if err != nil {
			return nil, nil, err
		}
		var stored *block.StoredBlock
		for _, pack := range values {
			value, ok := pack[key]
			if !ok {
				continue
			}
			candidate, err := block.DecodeBlockObject(value)
			if err != nil {
				return nil, nil, &block.CorruptError{Ref: block.NewBlockRef(h)}
			}
			stored = preferBlock(stored, candidate)
		}

		// Prefer the batch on equal sources and verify the chosen bytes once.
		stored = preferBlock(stored, batch[key])
		if err := block.VerifyRead(block.NewBlockRef(h), stored.Data, ""); err != nil {
			return nil, nil, err
		}
		return h, stored, nil
	})
}

// preferBlock keeps the copy with asserted refs over listed refs over unknown
// refs, choosing the candidate on a tie. Pack reads and pack batches only list
// refs, and writing a pack never persists an assertion.
func preferBlock(current, candidate *block.StoredBlock) *block.StoredBlock {
	if candidate == nil || current != nil && current.RefsSource > candidate.RefsSource {
		return current
	}
	return candidate
}

// replace publishes output, which may be nil, then deletes the entries of
// packfiles ids and then the packfiles. Deleting every entry first keeps each
// listed entry's packfile in place.
func (s *PackStore) replace(ctx context.Context, output *packfile.PackfileEntry, ids []string) error {
	// Publish the output before its inputs disappear.
	if output != nil {
		s.updateEntries([]*packfile.PackfileEntry{output}, nil)
	}

	// Unlist the inputs, then delete their packfiles.
	if err := s.deleteObjects(ctx, entryDir, ids); err != nil {
		return err
	}
	s.updateEntries(nil, ids)
	return s.deleteObjects(ctx, packDir, ids)
}

// readPack reads packfile id whole and returns its block values by key.
func (s *PackStore) readPack(ctx context.Context, id string) (map[string][]byte, error) {
	// Fetch the packfile object body from the S3 bucket.
	body, err := s.client.GetObject(ctx, s.bucket, s.prefix+packDir+id)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile "+id)
	}

	// Read the packfile bytes and close the object body.
	data, err := io.ReadAll(body)
	_ = body.Close()
	s.reads.downloaded(readWhole, len(data))
	if err != nil {
		return nil, errors.Wrap(err, "read packfile "+id)
	}

	// Open a kvfile reader over the packfile bytes.
	rd, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return nil, errors.Wrap(err, "open packfile "+id)
	}

	// Scan every key-value pair into the returned map.
	values := make(map[string][]byte)
	err = rd.ScanPrefix(nil, func(key, value []byte) error {
		values[string(key)] = value
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "scan packfile "+id)
	}
	return values, nil
}

// deleteObjects deletes every version of the object dir+id for each id.
func (s *PackStore) deleteObjects(ctx context.Context, dir string, ids []string) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchConcurrency)
	for _, id := range ids {
		eg.Go(func() error {
			return s.client.DeleteObject(egCtx, s.bucket, s.prefix+dir+id)
		})
	}
	return errors.Wrap(eg.Wait(), "delete replaced "+strings.TrimSuffix(dir, "/"))
}

// packTier returns the compaction tier of a packfile, or -1 when it is final.
// The unit that weighs 1 is at most the store's MaxPackBytes, so the merge of
// compactFanout packfiles of one tier fits one packfile.
func (s *PackStore) packTier(entry *packfile.PackfileEntry) int {
	weight := max(
		float64(entry.GetSizeBytes())/float64(min(compactMaxBytes, s.maxPackBytes)),
		float64(entry.GetBlockCount())/float64(compactMaxBlocks),
	)
	bound := math.Pow(compactFanout, -compactTiers)
	for tier := range compactTiers {
		if weight < bound {
			return tier
		}
		bound *= compactFanout
	}
	return -1
}
