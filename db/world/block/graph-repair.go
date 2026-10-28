package world_block

import (
	"bytes"
	"context"
	"slices"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	hidalgo "github.com/s4wave/spacewave/db/kvtx/hidalgo"
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
			got, err := t.readGraphRange(ctx, tree, root, hole.Low, hole.High, apply)
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
// World at root, failing when any page in the range is missing. With copyValues set,
// values kept in their own blocks are written again through into, so the keys
// stay readable in into without the older World's blocks; without it nothing
// is written, which a read-only transaction requires.
func (t *WorldState) readGraphRange(
	ctx context.Context,
	into *kvtx_block_okra.Tx,
	root *block.BlockRef,
	low, high []byte,
	copyValues bool,
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
	if !copyValues {
		return entries, nil
	}
	return into.CopyEntries(ctx, tree, entries)
}

// ReindexGraphTree repairs a graph tree whose missing pages all hold quad
// postings: it drops the missing ranges, then rebuilds every posting index
// from the primitive log, which keeps node values, reference counts, graph
// size and primitive IDs. It refuses when a missing page could hold anything
// but postings, since those cannot be rebuilt from the rest of the graph.
// Without apply it only reports.
func (t *WorldState) ReindexGraphTree(ctx context.Context, apply bool) (*GraphRepair, error) {
	// Apply held graph writes so the walk sees the current tree.
	if err := t.flushGraphWrites(ctx); err != nil {
		return nil, errors.Wrap(err, "graph write flush")
	}
	tree, ok := t.graphTree.(*kvtx_block_okra.Tx)
	if !ok {
		return nil, errors.Errorf("graph tree is %T, not an okra tree", t.graphTree)
	}
	qs, ok := t.graphHd.QuadStore.(*cayley_kv.QuadStore)
	if !ok {
		return nil, errors.Errorf("graph store is %T, not a kv quad store", t.graphHd.QuadStore)
	}
	layout := qs.Layout()

	// Collect every readable key and the missing pages.
	entries, holes, err := tree.Salvage(ctx, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "walk graph tree")
	}
	report := &GraphRepair{Keys: len(entries), Holes: holes, Fills: make([]GraphHoleFill, len(holes))}

	// Require each missing range to lie inside one posting bucket.
	for i, hole := range holes {
		if bucket := postingBucketOf(layout, hole.Low, hole.High); bucket == "" {
			report.Fills[i].Tried = append(report.Fills[i].Tried, "range is not inside one posting bucket")
		} else {
			report.Fills[i].Tried = append(report.Fills[i].Tried, "postings of index "+bucket+", rebuilt from the log")
		}
	}
	if !apply {
		return report, nil
	}
	for i, hole := range holes {
		if postingBucketOf(layout, hole.Low, hole.High) == "" {
			return report, errors.Errorf("missing page %s may hold more than postings", holes[i].Ref.MarshalString())
		}
	}

	// Drop the missing ranges, then rebuild the postings from the log
	// through the same store stack the graph handle uses.
	if err := tree.ReplaceAllEntries(ctx, entries); err != nil {
		return report, errors.Wrap(err, "replace graph tree")
	}
	var store kvtx.TxOps = tree
	if t.graphOps != nil {
		store = t.graphOps
	}
	rebuilder, err := cayley_kv.NewIndexRebuilder(flat.Upgrade(hidalgo.NewKV(kvtx.NewTxStore(store))), layout)
	if err != nil {
		return report, err
	}
	if _, err := rebuilder.Rebuild(ctx); err != nil {
		return report, errors.Wrap(err, "rebuild graph indexes")
	}
	if err := t.flushGraphWrites(ctx); err != nil {
		return report, errors.Wrap(err, "graph write flush")
	}
	report.Applied = true
	return report, nil
}

// postingBucketOf returns the posting bucket of layout whose flat keys hold
// all of [low, high), or empty when there is none.
func postingBucketOf(layout []cayley_kv.QuadIndex, low, high []byte) string {
	if len(low) == 0 || len(high) == 0 {
		return ""
	}
	for _, index := range layout {
		name := postingBucketName(index)
		first := []byte(name + "/")
		past := []byte(name + "0")
		if bytes.Compare(low, first) >= 0 && bytes.Compare(high, past) <= 0 {
			return name
		}
	}
	return ""
}

// postingBucketName returns the name of the posting bucket of index, as the
// kv quad store names it: the prefix byte of each direction.
func postingBucketName(index cayley_kv.QuadIndex) string {
	buf := make([]byte, len(index.Dirs))
	for i, d := range index.Dirs {
		buf[i] = d.Prefix()
		if buf[i] == 0 {
			buf[i] = '0'
		}
	}
	return string(buf)
}
