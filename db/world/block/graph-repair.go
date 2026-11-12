package world_block

import (
	"bytes"
	"context"
	"iter"
	"time"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	hidalgo "github.com/s4wave/spacewave/db/kvtx/hidalgo"
	"github.com/s4wave/spacewave/db/tx"
)

// GraphRepair reports a repair of the World's graph tree.
type GraphRepair struct {
	// Keys is the number of keys the current tree can read.
	Keys uint64
	// Holes are the pages of the current tree that are missing.
	Holes []kvtx_block_okra.SalvageHole
	// Fills are the keys found for each hole, indexed like Holes.
	Fills []GraphHoleFill
	// Applied is set when the rebuilt tree replaced the graph tree.
	Applied bool
	// Seqno is the World's sequence number, set by GraphTreeHolesAt.
	Seqno uint64
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
	graphTree, err := t.graphKvTx(ctx)
	if err != nil {
		return nil, err
	}
	tree, ok := graphTree.(*kvtx_block_okra.Tx)
	if !ok {
		return nil, errors.Errorf("graph tree is %T, not an okra tree", graphTree)
	}

	// Count the readable keys and find the missing pages.
	keys, holes, err := tree.SalvageHoles(ctx, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "walk graph tree")
	}
	report := &GraphRepair{Keys: keys, Holes: holes, Fills: make([]GraphHoleFill, len(holes))}

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

	// Rebuild the tree from the readable keys, with each hole's fill in its
	// place. The walk meets the holes in the order it reported them.
	next := 0
	entries := salvageEntries(ctx, tree, func(hole kvtx_block_okra.SalvageHole) ([]kvtx_block_okra.BuildEntry, error) {
		if next >= len(holes) || !hole.Ref.EqualsRef(holes[next].Ref) {
			return nil, errors.Errorf("graph tree changed during repair at missing page %s", hole.Ref.MarshalString())
		}
		fill := report.Fills[next]
		next++
		return fill.entries, nil
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
	tree, _, err := t.openGraphTreeAt(ctx, root)
	if err != nil {
		return nil, err
	}
	defer tree.Discard()

	// Read the range, which must have no missing page.
	var entries []kvtx_block_okra.BuildEntry
	var missing *block.BlockRef
	err = tree.Salvage(ctx, low, high, func(entry kvtx_block_okra.BuildEntry) bool {
		entries = append(entries, entry)
		return true
	}, func(hole kvtx_block_okra.SalvageHole) bool {
		missing = hole.Ref
		return false
	})
	if err != nil {
		return nil, err
	}
	if missing != nil {
		return nil, errors.Errorf("page %s missing in range", missing.MarshalString())
	}
	if !copyValues {
		return entries, nil
	}
	return into.CopyEntries(ctx, tree, entries)
}

// GraphTreeHolesAt walks the whole graph tree of the World at root and reports
// its readable keys and missing pages, writing nothing.
func (t *WorldState) GraphTreeHolesAt(ctx context.Context, root *block.BlockRef) (*GraphRepair, error) {
	// Open the tree at root.
	tree, seqno, err := t.openGraphTreeAt(ctx, root)
	if err != nil {
		return nil, err
	}
	defer tree.Discard()

	// Count the readable keys and find the missing pages.
	keys, holes, err := tree.SalvageHoles(ctx, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "walk graph tree")
	}
	return &GraphRepair{Keys: keys, Holes: holes, Fills: make([]GraphHoleFill, len(holes)), Seqno: seqno}, nil
}

// openGraphTreeAt opens the graph tree of the World at root read-only, over
// the store of this World, and returns it with the World's sequence number.
// Discard the returned tree when done.
func (t *WorldState) openGraphTreeAt(ctx context.Context, root *block.BlockRef) (*kvtx_block_okra.Tx, uint64, error) {
	// A read-only state has no block transaction of its own, so take the
	// bucket's transformer from the cursor's transaction.
	_, bcs := block.NewTransaction(t.store, t.bcs.GetTransaction().GetTransformer(), root, nil)
	w, err := block.UnmarshalBlock[World](ctx, bcs)
	if err != nil {
		return nil, 0, errors.Wrap(err, "load World root")
	}
	gtx, err := kvtx_block.BuildKvTransaction(ctx, bcs.FollowSubBlock(2), false)
	if err != nil {
		return nil, 0, errors.Wrap(err, "open graph tree")
	}
	tree, ok := gtx.(*kvtx_block_okra.Tx)
	if !ok {
		gtx.Discard()
		return nil, 0, errors.Errorf("graph tree is %T, not an okra tree", gtx)
	}
	return tree, w.GetLastChange().GetSeqno(), nil
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
	graphTree, err := t.graphKvTx(ctx)
	if err != nil {
		return nil, err
	}
	tree, ok := graphTree.(*kvtx_block_okra.Tx)
	if !ok {
		return nil, errors.Errorf("graph tree is %T, not an okra tree", graphTree)
	}
	qs, ok := t.graphHd.QuadStore.(*cayley_kv.QuadStore)
	if !ok {
		return nil, errors.Errorf("graph store is %T, not a kv quad store", t.graphHd.QuadStore)
	}
	layout := qs.Layout()

	// Count the readable keys and find the missing pages.
	keys, holes, err := tree.SalvageHoles(ctx, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "walk graph tree")
	}
	report := &GraphRepair{Keys: keys, Holes: holes, Fills: make([]GraphHoleFill, len(holes))}

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

	// Drop the missing ranges.
	entries := salvageEntries(ctx, tree, nil)
	if err := tree.ReplaceAllEntries(ctx, entries); err != nil {
		return report, errors.Wrap(err, "replace graph tree")
	}

	// Rebuild the postings from the log through the same store stack the
	// graph handle uses.
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

// salvageEntries streams the readable entries of tree in key order, with the
// entries fill returns for each missing page in its place. A nil fill drops
// the missing pages.
func salvageEntries(
	ctx context.Context,
	tree *kvtx_block_okra.Tx,
	fill func(kvtx_block_okra.SalvageHole) ([]kvtx_block_okra.BuildEntry, error),
) iter.Seq2[kvtx_block_okra.BuildEntry, error] {
	return func(yield func(kvtx_block_okra.BuildEntry, error) bool) {
		stopped := false
		var fillErr error
		err := tree.Salvage(ctx, nil, nil, func(entry kvtx_block_okra.BuildEntry) bool {
			stopped = !yield(entry, nil)
			return !stopped
		}, func(hole kvtx_block_okra.SalvageHole) bool {
			if fill == nil {
				return true
			}
			var entries []kvtx_block_okra.BuildEntry
			entries, fillErr = fill(hole)
			if fillErr != nil {
				return false
			}
			for _, entry := range entries {
				if stopped = !yield(entry, nil); stopped {
					return false
				}
			}
			return true
		})
		if err == nil {
			err = fillErr
		}
		if err != nil && !stopped {
			yield(kvtx_block_okra.BuildEntry{}, err)
		}
	}
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

// DumpGraphTree walks the graph tree of this World in key order, calling
// entry with each readable key and its value and hole with each missing page,
// and writes nothing. A value the tree keeps in a page is passed as it is; a
// value kept in its own block is read after the walk, in key order among
// those values only. The walk holds only the pages on its current path.
func (t *WorldState) DumpGraphTree(
	ctx context.Context,
	entry func(key, value []byte) error,
	hole func(kvtx_block_okra.SalvageHole) error,
) error {
	// Open the graph tree.
	graphTree, err := t.graphKvTx(ctx)
	if err != nil {
		return err
	}
	tree, ok := graphTree.(*kvtx_block_okra.Tx)
	if !ok {
		return errors.Errorf("graph tree is %T, not an okra tree", graphTree)
	}

	// Walk the readable entries and the holes; stop at the first error.
	var cbErr error
	var refKeys [][]byte
	err = tree.Salvage(ctx, nil, nil, func(e kvtx_block_okra.BuildEntry) bool {
		switch {
		case e.ValueBlob != nil && e.ValueBlob.GetBlobType() == blob.BlobType_BlobType_RAW:
			cbErr = entry(e.Key, e.ValueBlob.GetRawData())
		case e.ValueBlob == nil && e.ValueRef.GetEmpty():
			cbErr = entry(e.Key, nil)
		default:
			refKeys = append(refKeys, bytes.Clone(e.Key))
		}
		return cbErr == nil
	}, func(h kvtx_block_okra.SalvageHole) bool {
		cbErr = hole(h)
		return cbErr == nil
	})
	if err == nil {
		err = cbErr
	}
	if err != nil {
		return err
	}

	// Read the values kept in their own blocks.
	for _, key := range refKeys {
		value, found, err := tree.Get(ctx, key)
		if err != nil {
			// A missing value block is a hole of the one key.
			if ctx.Err() != nil {
				return errors.Wrapf(err, "read value of key %q", key)
			}
			if herr := hole(kvtx_block_okra.SalvageHole{Low: key, High: append(bytes.Clone(key), 0), Keys: 1, Err: err}); herr != nil {
				return herr
			}
			continue
		}
		if !found {
			return errors.Errorf("key %q not found while reading its value", key)
		}
		if err := entry(key, value); err != nil {
			return err
		}
	}
	return nil
}

// DumpObjectTree walks the object tree of this World in key order, calling
// object with each readable object's key, root ref and revision, record with
// each key whose record would not read, and hole with each missing page of
// the tree. A record block the store lacks is fetched from peers, which may
// never answer, so each record read ends after recordTimeout. It writes nothing.
func (t *WorldState) DumpObjectTree(
	ctx context.Context,
	recordTimeout time.Duration,
	object func(key string, rootRef *bucket.ObjectRef, rev uint64) error,
	record func(key string, err error) error,
	hole func(kvtx_block_okra.SalvageHole) error,
) error {
	tree, ok := t.objTree.(*kvtx_block_okra.Tx)
	if !ok {
		return errors.Errorf("object tree is %T, not an okra tree", t.objTree)
	}

	// Collect the readable keys and report the holes.
	var cbErr error
	var keys []string
	err := tree.Salvage(ctx, nil, nil, func(e kvtx_block_okra.BuildEntry) bool {
		if key, ok := bytes.CutPrefix(e.Key, []byte(objectKeyPrefix)); ok {
			keys = append(keys, string(key))
		}
		return true
	}, func(h kvtx_block_okra.SalvageHole) bool {
		cbErr = hole(h)
		return cbErr == nil
	})
	if err == nil {
		err = cbErr
	}
	if err != nil {
		return err
	}

	// Read each object's record.
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		rctx, cancel := context.WithTimeout(ctx, recordTimeout)
		obj, found, err := t.getObjectRootRef(rctx, key)
		cancel()
		if err == nil && !found {
			err = errors.New("record not found")
		}
		if err != nil {
			if err := record(key, err); err != nil {
				return err
			}
			continue
		}
		if err := object(key, t.externalObjectRef(obj.GetRootRef()), obj.GetRev()); err != nil {
			return err
		}
	}
	return nil
}

// PutStoredBlocks writes encoded blocks, each verified against its ref,
// through the store this World writes through, so the commit whose root
// reaches them adopts them as it adopts the World's own writes. Each entry
// lists its outgoing refs and declares them complete. Use it to carry blocks
// of another copy of this World, which share its transforms.
func (t *WorldState) PutStoredBlocks(ctx context.Context, entries []*block.PutBatchEntry) error {
	// Require an open write transaction.
	if !t.write {
		return tx.ErrNotWrite
	}
	if err := t.discardErr(); err != nil {
		return err
	}

	// Require each entry's ref and data, and declare its refs complete.
	for _, e := range entries {
		if e.Ref.GetEmpty() || len(e.Data) == 0 || e.Tombstone {
			return errors.New("a stored block needs its ref and data")
		}
		e.RefsKnown = true
	}

	// Write them through the World's store.
	_, err := t.store.PutBlockBatch(ctx, entries)
	return err
}

// IsWrite reports whether this World state accepts writes.
func (t *WorldState) IsWrite() bool {
	return t.write
}

// ReplaceObjectRecord writes a new record for the object at key with rootRef,
// without reading the record it replaces, so an object whose record block the
// store lacks gets a readable record again. The object keeps its graph quads.
// Appends an OBJECT_SET change to the changelog.
func (t *WorldState) ReplaceObjectRecord(ctx context.Context, key string, rootRef *bucket.ObjectRef) error {
	// Require an open write transaction.
	if !t.write {
		return tx.ErrNotWrite
	}
	if err := t.discardErr(); err != nil {
		return err
	}

	// Write the new record over whatever the tree holds at key.
	k := []byte(objectKeyPrefix + key)
	obj := NewObject(key, t.localObjectRef(rootRef))
	nbcs := t.bcs.Detach(false)
	nbcs.ClearAllRefs()
	nbcs.SetBlock(obj, true)
	if err := t.objTree.SetCursorAtKey(ctx, k, nbcs, false); err != nil {
		return err
	}
	t.markObjectWritten(key, nbcs)
	t.noteSetRoot(key, obj.GetRootRef())

	// Record the write in the changelog and the existence memo.
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_SET,
	})
	if err != nil {
		return err
	}
	if changeBcs != nil {
		changeBcs.SetRef(5, nbcs)
		t.noteLoggedRoot(obj.GetRootRef())
	}
	t.markObjectExists(key)
	return nil
}

// SalvageObjectRange walks the object keys in [low, high), keys without the
// object prefix, calling entry for each readable key and hole for each page
// that reads as missing. Empty bounds are open.
func (t *WorldState) SalvageObjectRange(ctx context.Context, low, high string, entry func(key string) error, hole func(kvtx_block_okra.SalvageHole) error) error {
	tree, ok := t.objTree.(*kvtx_block_okra.Tx)
	if !ok {
		return errors.Errorf("object tree is %T, not an okra tree", t.objTree)
	}
	var lo, hi []byte
	if low != "" {
		lo = []byte(objectKeyPrefix + low)
	}
	if high != "" {
		hi = []byte(objectKeyPrefix + high)
	} else {
		hi = []byte(objectKeyPrefix[:len(objectKeyPrefix)-1] + string(rune(objectKeyPrefix[len(objectKeyPrefix)-1]+1)))
	}
	var cbErr error
	err := tree.Salvage(ctx, lo, hi, func(e kvtx_block_okra.BuildEntry) bool {
		if key, ok := bytes.CutPrefix(e.Key, []byte(objectKeyPrefix)); ok {
			cbErr = entry(string(key))
		}
		return cbErr == nil
	}, func(h kvtx_block_okra.SalvageHole) bool {
		cbErr = hole(h)
		return cbErr == nil
	})
	if err == nil {
		err = cbErr
	}
	return err
}

// GetGraphKey reads one raw key of the graph tree.
func (t *WorldState) GetGraphKey(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Open the graph tree and read the key.
	graphTree, err := t.graphKvTx(ctx)
	if err != nil {
		return nil, false, err
	}
	tree, ok := graphTree.(*kvtx_block_okra.Tx)
	if !ok {
		return nil, false, errors.Errorf("graph tree is %T, not an okra tree", graphTree)
	}
	return tree.Get(ctx, key)
}

// ObjectRecordReadable reports whether the object at key has a readable
// record, reading it under ctx.
func (t *WorldState) ObjectRecordReadable(ctx context.Context, key string) (bool, error) {
	_, found, err := t.getObjectRootRef(ctx, key)
	return found, err
}
