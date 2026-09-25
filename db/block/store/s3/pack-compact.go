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
	// compactMaxBytes is the packfile size that weighs 1.
	compactMaxBytes = 16 << 20
	// compactMaxBlocks is the packfile block count that weighs 1.
	compactMaxBlocks = writer.DefaultMaxBlocksPerPack
)

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
// A packfile's weight is the larger of its share of compactMaxBytes and of
// compactMaxBlocks. Weights below 1/512, 1/64, and 1/8 form tiers 0 to 2; the
// rest are final. The compactFanout packfiles of one tier merge into a
// packfile of weight below 1, so each block is rewritten at most compactTiers
// times and at most compactFanout-1 packfiles wait in each tier.
//
// The pass lists the entries first to see the packfiles other writers added. A
// merge deletes only inputs whose every block its output holds, so concurrent
// passes on several devices lose no block.
func (s *PackStore) compact(ctx context.Context) error {
	release, err := s.compactMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	if err := s.listEntries(ctx, s.listings.Load()); err != nil {
		return err
	}
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
			if tier := packTier(entry); tier >= 0 {
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

// merge writes the blocks of inputs as one packfile, then deletes the inputs'
// entries and then their packfiles. Returns ErrNotFound when an input is gone.
//
// The merged packfile holds the blocks in key order, so merging the same
// inputs anywhere writes the same packfile id.
func (s *PackStore) merge(ctx context.Context, inputs []*packfile.PackfileEntry) error {
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

	values := make(map[string][]byte)
	for _, pack := range packs {
		maps.Copy(values, pack)
	}
	keys := slices.Sorted(maps.Keys(values))
	i := 0
	merged, err := s.writePack(ctx, func() (*hash.Hash, *block.StoredBlock, error) {
		if i == len(keys) {
			return nil, nil, nil
		}
		key := keys[i]
		i++
		ref, stored, err := packfile.DecodeBlockValue([]byte(key), values[key])
		if err != nil {
			return nil, nil, err
		}
		return ref.GetHash(), stored, nil
	})
	if err != nil {
		return errors.Wrap(err, "write merged packfile")
	}
	if merged == nil {
		return errors.New("merged packfiles hold no blocks")
	}
	s.updateEntries([]*packfile.PackfileEntry{merged}, nil)

	ids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if input.GetId() != merged.GetId() {
			ids = append(ids, input.GetId())
		}
	}
	if err := s.deleteObjects(ctx, entryDir, ids); err != nil {
		return err
	}
	s.updateEntries(nil, ids)
	return s.deleteObjects(ctx, packDir, ids)
}

// readPack reads packfile id whole and returns its block values by key.
func (s *PackStore) readPack(ctx context.Context, id string) (map[string][]byte, error) {
	body, err := s.client.GetObject(ctx, s.bucket, s.prefix+packDir+id)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile "+id)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "read packfile "+id)
	}
	rd, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return nil, errors.Wrap(err, "open packfile "+id)
	}
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

// deleteObjects deletes the object dir+id for each id.
func (s *PackStore) deleteObjects(ctx context.Context, dir string, ids []string) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchConcurrency)
	for _, id := range ids {
		eg.Go(func() error {
			return s.client.DeleteObject(egCtx, s.bucket, s.prefix+dir+id)
		})
	}
	return errors.Wrap(eg.Wait(), "delete merged "+strings.TrimSuffix(dir, "/"))
}

// packTier returns the compaction tier of a packfile, or -1 when it is final.
func packTier(entry *packfile.PackfileEntry) int {
	weight := max(
		float64(entry.GetSizeBytes())/compactMaxBytes,
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
