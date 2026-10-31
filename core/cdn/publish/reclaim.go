package publish

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

// reclaimMaxReplaced is how many packs one replacement push may supersede.
const reclaimMaxReplaced = 32

// reclaimGroupPacks is how many mostly dead packs one replacement supersedes,
// leaving a replaced ID for a borrowed pack.
const reclaimGroupPacks = reclaimMaxReplaced - 1

// reclaimConcurrency bounds the concurrent pack index reads.
const reclaimConcurrency = 8

// ReadBlock returns the stored bytes and refs of a live block.
type ReadBlock func(ctx context.Context, h *hash.Hash) (*block.StoredBlock, error)

// reclaimPack is the liveness of the blocks of one published pack.
type reclaimPack struct {
	// entry is the catalog entry of the pack.
	entry *packfile.PackfileEntry
	// live are the keys of the pack's live blocks.
	live []string
	// total is the value bytes of every block.
	total uint64
	// dead is the value bytes of the dead blocks.
	dead uint64
}

// Reclaim rewrites the destination's packs that are at least half dead. live
// lists every block the destination still serves, in the order to write them
// again; a block missing from live reads as dead, so live must be the
// complete closure of every served root. Each group of mostly dead packs is
// superseded by one pack of their live blocks, which read returns, so the
// pass downloads only pack indexes. The service deletes superseded packs
// after a grace period that lets readers of an older root pointer finish.
// check, when set, runs before each replacement and stops the pass with its
// error. Returns the dead value bytes dropped.
//
// The destination must have one publisher at a time: a block judged dead is
// dropped even if another writer dedups against it before the replacement.
func Reclaim(
	ctx context.Context,
	opts Options,
	live []*hash.Hash,
	read ReadBlock,
	check func(context.Context) error,
) (uint64, error) {
	// A destination that keeps every pack replaces none.
	if opts.KeepPacks {
		return 0, errors.New("the destination keeps every pack")
	}

	// Judge every published pack from its key index.
	liveKeys := make(map[string]struct{}, len(live))
	for _, h := range live {
		liveKeys[string(packfile.BlockKey(h))] = struct{}{}
	}
	packs, err := judgePacks(ctx, opts, liveKeys)
	if err != nil {
		return 0, err
	}

	// Split the mostly dead packs from the kept ones, smallest kept first so
	// a group with no live block borrows the cheapest one.
	var dead, kept []*reclaimPack
	for _, pack := range packs {
		if pack.dead != 0 && pack.dead*2 >= pack.total {
			dead = append(dead, pack)
		} else {
			kept = append(kept, pack)
		}
	}
	slices.SortFunc(kept, func(a, b *reclaimPack) int { return cmp.Compare(a.total, b.total) })

	// Replace each group, borrowing a kept pack when the group has no live
	// block left to carry the replacement.
	var dropped uint64
	written := make(map[string]struct{})
	for group := range slices.Chunk(dead, reclaimGroupPacks) {
		// Stop when the caller's check fails.
		if check != nil {
			if err := check(ctx); err != nil {
				return dropped, err
			}
		}

		// Collect the live blocks the group holds that no replacement wrote yet.
		group = slices.Clone(group)
		need := neededKeys(group, written)
		for len(need) == 0 && len(kept) != 0 && len(group) < reclaimMaxReplaced {
			group, kept = append(group, kept[0]), kept[1:]
			need = neededKeys(group, written)
		}
		if len(need) == 0 {
			continue
		}

		// Write them and supersede the group.
		if err := replaceGroup(ctx, opts, group, need, live, read); err != nil {
			return dropped, err
		}
		for key := range need {
			written[key] = struct{}{}
		}
		for _, pack := range group {
			dropped += pack.dead
		}
	}
	return dropped, nil
}

// judgePacks reads the key index of each pack in the destination catalog and
// sums its live and dead value bytes against live.
func judgePacks(ctx context.Context, opts Options, live map[string]struct{}) ([]*reclaimPack, error) {
	// List the catalog.
	entries, err := FetchPackEntries(ctx, opts.Client, opts.DstSpaceID)
	if err != nil {
		return nil, err
	}

	// Read the indexes concurrently through anonymous ranged CDN reads.
	open := cdn_bstore.NewAnonymousOpener(nil, opts.CdnBaseURL, opts.DstSpaceID)
	packs := make([]*reclaimPack, len(entries))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(reclaimConcurrency)
	for i, entry := range entries {
		eg.Go(func() error {
			// Read the pack's key index.
			size := entry.GetSizeBytes()
			rd, err := open(entry.GetId(), int64(size)) //nolint:gosec // catalog packs are at most writer.DefaultMaxPackBytes.
			if err != nil {
				return err
			}
			defer rd.Close()
			index, err := packfile.ReadIndex(rd.ReaderAt(egCtx), size)
			if err != nil {
				return errors.Wrap(err, entry.GetId())
			}

			// Sum the value bytes of every block and of the dead ones.
			pack := &reclaimPack{entry: entry}
			for _, ie := range index {
				pack.total += ie.GetSize()
				if _, ok := live[string(ie.GetKey())]; ok {
					pack.live = append(pack.live, string(ie.GetKey()))
				} else {
					pack.dead += ie.GetSize()
				}
			}
			packs[i] = pack
			return nil
		})
	}
	return packs, eg.Wait()
}

// neededKeys returns the live keys of group not in written.
func neededKeys(group []*reclaimPack, written map[string]struct{}) map[string]struct{} {
	need := make(map[string]struct{})
	for _, pack := range group {
		for _, key := range pack.live {
			if _, ok := written[key]; !ok {
				need[key] = struct{}{}
			}
		}
	}
	return need
}

// replaceGroup uploads the blocks of need in live order and supersedes the
// packs of group with the last uploaded pack, once the others are durable.
func replaceGroup(
	ctx context.Context,
	opts Options,
	group []*reclaimPack,
	need map[string]struct{},
	live []*hash.Hash,
	read ReadBlock,
) error {
	// Feed the needed blocks in live order to keep file blocks together.
	index := 0
	next := func() (*hash.Hash, *block.StoredBlock, error) {
		for index < len(live) {
			h := live[index]
			index++
			if _, ok := need[string(packfile.BlockKey(h))]; ok {
				stored, err := read(ctx, h)
				return h, stored, err
			}
		}
		return nil, nil, nil
	}

	// Supersede the group once its live blocks are durable.
	replaced := make([]string, len(group))
	for i, pack := range group {
		replaced[i] = pack.entry.GetId()
	}
	return errors.Wrap(pushSuperseding(ctx, opts, next, replaced), "replace reclaimed packs")
}

// pushSuperseding uploads the blocks of iter in order and supersedes the
// replaced packs, at most reclaimMaxReplaced of them, with the last uploaded
// pack once the others are durable.
func pushSuperseding(ctx context.Context, opts Options, iter writer.BlockIterator, replaced []string) error {
	// Upload every chunk but the last as an ordinary pack, holding one back.
	if len(replaced) > reclaimMaxReplaced {
		return errors.Errorf("one push supersedes at most %d packs, not %d", reclaimMaxReplaced, len(replaced))
	}
	var held []byte
	var heldBloom []byte
	_, err := delta.EmitDeltaChunks(ctx, opts.DstSpaceID, iter, delta.DefaultMaxChunkBytes, func(ctx context.Context, _ int, entry *packfile.PackfileEntry, data []byte) error {
		if held != nil {
			if err := PushPackData(ctx, opts, held, heldBloom); err != nil {
				return err
			}
		}
		held, heldBloom = data, entry.GetBloomFilter()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "upload packs")
	}
	if held == nil {
		return errors.New("no blocks to carry the replacement")
	}

	// Supersede the replaced packs with the last chunk.
	return PushReplacementData(ctx, opts, held, heldBloom, replaced)
}
