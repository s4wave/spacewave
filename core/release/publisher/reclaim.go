package publisher

import (
	"cmp"
	"context"
	"slices"

	"github.com/pkg/errors"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/packfile"
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

// reclaim rewrites the destination's packs that are at least half dead once
// the checkpoint whose World is head has been posted. blocks is the complete
// closure of head. Each group of mostly dead packs is superseded by one pack
// of their live blocks, written from blocks, so the pass downloads only pack
// indexes. Returns the dead value bytes dropped.
//
// The destination must have one publisher at a time: a block judged dead is
// dropped even if another writer dedups against it before the replacement.
// The pass stops when the destination head moves away from head.
func reclaim(ctx context.Context, opts cdn_publish.Options, head *bucket.ObjectRef, blocks []packedBlock) (uint64, error) {
	// Index the live blocks and require the closure to hold every stored ref,
	// since a missed subtree would read as dead.
	live := make(map[string]struct{}, len(blocks))
	for _, entry := range blocks {
		live[string(packfile.BlockKey(entry.ref.GetHash()))] = struct{}{}
	}
	for _, entry := range blocks {
		for _, ref := range entry.stored.GetRefs() {
			if ref.GetEmpty() {
				continue
			}
			if _, ok := live[string(packfile.BlockKey(ref.GetHash()))]; !ok {
				return 0, errors.Errorf("release closure lacks %s referenced by %s", ref.MarshalString(), entry.ref.MarshalString())
			}
		}
	}

	// The checkpoint holds head without its bucket binding.
	want := head.CloneVT()
	want.BucketId = ""

	// Judge every published pack from its key index.
	packs, err := judgePacks(ctx, opts, live)
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
		// Stop when another publication moved the head.
		current, err := cdn_publish.FetchSourceHeadRef(ctx, opts.Client, opts.DstSpaceID)
		if err != nil {
			return dropped, errors.Wrap(err, "read destination head")
		}
		if !current.EqualVT(want) {
			return dropped, errors.New("destination head moved during reclaim")
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
		if err := replaceGroup(ctx, opts, group, need, blocks); err != nil {
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
func judgePacks(ctx context.Context, opts cdn_publish.Options, live map[string]struct{}) ([]*reclaimPack, error) {
	// List the catalog, which now includes the packs of this publication.
	entries, err := cdn_publish.FetchPackEntries(ctx, opts.Client, opts.DstSpaceID)
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

// replaceGroup uploads the blocks of need in closure order and supersedes the
// packs of group with the last uploaded pack, once the others are durable.
func replaceGroup(ctx context.Context, opts cdn_publish.Options, group []*reclaimPack, need map[string]struct{}, blocks []packedBlock) error {
	// Feed the needed blocks in closure order to keep file blocks together.
	index := 0
	next := func() (*hash.Hash, *block.StoredBlock, error) {
		for index < len(blocks) {
			entry := blocks[index]
			index++
			if _, ok := need[string(packfile.BlockKey(entry.ref.GetHash()))]; ok {
				return entry.ref.GetHash(), entry.stored, nil
			}
		}
		return nil, nil, nil
	}

	// Upload every chunk but the last as an ordinary pack, holding one back.
	var held []byte
	var heldBloom []byte
	_, err := delta.EmitDeltaChunks(ctx, opts.DstSpaceID, next, delta.DefaultMaxChunkBytes, func(ctx context.Context, _ int, entry *packfile.PackfileEntry, data []byte) error {
		if held != nil {
			if err := cdn_publish.PushPackData(ctx, opts, held, heldBloom); err != nil {
				return err
			}
		}
		held, heldBloom = data, entry.GetBloomFilter()
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "upload reclaimed packs")
	}

	// Supersede the group with the last chunk.
	replaced := make([]string, len(group))
	for i, pack := range group {
		replaced[i] = pack.entry.GetId()
	}
	return errors.Wrap(cdn_publish.PushReplacementData(ctx, opts, held, heldBloom, replaced), "replace reclaimed packs")
}
