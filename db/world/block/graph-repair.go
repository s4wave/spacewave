package world_block

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// GraphRepair reports a repair of the World's graph tree.
type GraphRepair struct {
	// Keys is the number of keys the current tree can read.
	Keys int
	// Holes are the pages of the current tree that are missing.
	Holes []kvtx_block_okra.SalvageHole
	// Fills are the keys found for each hole, indexed like Holes.
	Fills []GraphHoleFill
	// Applied is set when the rebuilt tree replaced the graph tree.
	Applied bool
}

// GraphHoleFill is what an older World held in the key range of one hole.
type GraphHoleFill struct {
	// Root is the World root the keys came from, or nil when no root held
	// the whole range.
	Root *block.BlockRef
	// Keys is the number of keys the range held in Root.
	Keys int
	// Tried are the roots that did not hold the whole range, with why.
	Tried []string
	// entries are the keys of the range.
	entries []kvtx_block_okra.BuildEntry
}

// RepairGraphTree finds the graph tree's missing pages and fills each one's
// key range from the first of fillRoots whose graph tree reads that whole
// range. With apply set it replaces the graph tree with every readable key
// plus the fills. It refuses to apply while a hole has no fill, unless
// dropHoles is set, which drops those ranges.
func (t *WorldState) RepairGraphTree(
	ctx context.Context,
	fillRoots []*block.BlockRef,
	apply bool,
	dropHoles bool,
) (*GraphRepair, error) {
	// Apply held graph writes so the walk sees the current tree.
	if err := t.flushGraphWrites(ctx); err != nil {
		return nil, errors.Wrap(err, "graph write flush")
	}
	tree, ok := t.graphTree.(*kvtx_block_okra.Tx)
	if !ok {
		return nil, errors.Errorf("graph tree is %T, not an okra tree", t.graphTree)
	}

	// Collect every readable key and the missing pages.
	entries, holes, err := tree.Salvage(ctx, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "walk graph tree")
	}
	report := &GraphRepair{Keys: len(entries), Holes: holes, Fills: make([]GraphHoleFill, len(holes))}

	// Fill each hole from the first older World that reads its whole range.
	for i, hole := range holes {
		fill := &report.Fills[i]
		for _, root := range fillRoots {
			got, err := t.readGraphRange(ctx, tree, root, hole.Low, hole.High)
			if err != nil {
				fill.Tried = append(fill.Tried, root.MarshalString()+": "+err.Error())
				continue
			}
			fill.Root, fill.Keys, fill.entries = root.Clone(), len(got), got
			break
		}
	}
	if !apply {
		return report, nil
	}

	// Refuse to drop a range unless asked.
	for i, fill := range report.Fills {
		if fill.Root == nil && !dropHoles {
			return report, errors.Errorf("no root holds the range of missing page %s", holes[i].Ref.MarshalString())
		}
	}

	// Merge the fills into the readable keys and replace the tree.
	for _, fill := range report.Fills {
		entries = append(entries, fill.entries...)
	}
	slices.SortFunc(entries, func(a, b kvtx_block_okra.BuildEntry) int {
		return bytes.Compare(a.Key, b.Key)
	})
	entries = slices.CompactFunc(entries, func(a, b kvtx_block_okra.BuildEntry) bool {
		return bytes.Equal(a.Key, b.Key)
	})
	if err := tree.ReplaceAllEntries(ctx, entries); err != nil {
		return report, errors.Wrap(err, "replace graph tree")
	}
	report.Applied = true
	return report, nil
}

// readGraphRange returns the keys in [low, high) of the graph tree of the
// World at root, failing when any page in the range is missing. Values kept in
// their own blocks are written again through into, so the keys stay readable
// in into without the older World's blocks.
func (t *WorldState) readGraphRange(
	ctx context.Context,
	into *kvtx_block_okra.Tx,
	root *block.BlockRef,
	low, high []byte,
) ([]kvtx_block_okra.BuildEntry, error) {
	// Open the older World in a read transaction over the same store.
	_, bcs := block.NewTransaction(t.store, t.btx.GetTransformer(), root, nil)
	if _, err := block.UnmarshalBlock[World](ctx, bcs); err != nil {
		return nil, errors.Wrap(err, "load World root")
	}
	gtx, err := kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(2), false)
	if err != nil {
		return nil, errors.Wrap(err, "open graph tree")
	}
	defer gtx.Discard()
	tree, ok := gtx.(*kvtx_block_okra.Tx)
	if !ok {
		return nil, errors.Errorf("graph tree is %T, not an okra tree", gtx)
	}

	// Read the range, which must have no missing page.
	entries, holes, err := tree.Salvage(ctx, low, high)
	if err != nil {
		return nil, err
	}
	if len(holes) != 0 {
		return nil, errors.Errorf("%d pages missing in range, first %s", len(holes), holes[0].Ref.MarshalString())
	}
	return into.CopyEntries(ctx, tree, entries)
}
