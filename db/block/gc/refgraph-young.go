package block_gc

import (
	"context"
	"encoding/binary"
	"errors"
	"slices"

	"github.com/s4wave/spacewave/db/kvtx"
)

// youngKey is the one record of the young log's range.
var youngKey = []byte{kindYoungHead}

// youngReadyKey is the record that reports the young log complete.
var youngReadyKey = []byte{kindYoungReady}

// YoungReady reports whether the young log is complete, so minor passes may
// run: an old block names only old blocks, and every later edge from one is in
// a record, as PromoteYoung describes. The record survives restarts. Only a
// store format whose every build logs young records may keep it, so a build
// without the log never opens a store that has it.
func (g *RefGraph) YoungReady(ctx context.Context) (bool, error) {
	var ready bool
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) (err error) {
		ready, err = tx.Exists(ctx, youngReadyKey)
		return err
	})
	return ready, err
}

// ClaimYoungReady marks the young log complete when the graph tracks no block,
// and reports whether it is complete. With no child list there is no old
// block, and every list written later is logged, so a store born empty is
// minor-ready from birth. The caller must run it only where every later write
// logs young records.
func (g *RefGraph) ClaimYoungReady(ctx context.Context) (bool, error) {
	var ready bool
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Keep a claim already made.
		found, err := tx.Exists(ctx, youngReadyKey)
		if err != nil || found {
			ready = found
			return err
		}

		// Claim only a graph without child lists.
		it := tx.Iterate(ctx, []byte{kindChildren}, false, false)
		tracked := it.Next()
		err = it.Err()
		it.Close()
		if err != nil || tracked {
			return err
		}
		ready = true
		return tx.Set(ctx, youngReadyKey, nil)
	})
	return ready, err
}

// youngRange is the young log: records lo up to hi.
type youngRange struct {
	// lo is the first record no pass promoted.
	lo uint64
	// hi is the sequence of the next record.
	hi uint64
}

// readYoungTx returns the young log's range, empty when no write logged one.
func readYoungTx(ctx context.Context, tx kvtx.TxOps) (youngRange, error) {
	// Read the range record.
	value, found, err := tx.Get(ctx, youngKey)
	if err != nil || !found {
		return youngRange{}, err
	}
	if len(value) != 16 {
		return youngRange{}, errYoungRecord
	}
	r := youngRange{lo: binary.BigEndian.Uint64(value), hi: binary.BigEndian.Uint64(value[8:])}
	if r.lo > r.hi {
		return youngRange{}, errYoungRecord
	}
	return r, nil
}

// writeYoungTx stores the young log's range.
func writeYoungTx(ctx context.Context, tx kvtx.TxOps, r youngRange) error {
	return tx.Set(ctx, youngKey, binary.BigEndian.AppendUint64(binary.BigEndian.AppendUint64(nil, r.lo), r.hi))
}

// youngRecordKey returns the key of the young log record seq.
func youngRecordKey(seq uint64) []byte {
	return binary.BigEndian.AppendUint64([]byte{kindYoung}, seq)
}

// youngRecord is one write's entry in the young log.
type youngRecord struct {
	// created lists the blocks whose child list the write created.
	created []string
	// changed lists the blocks whose existing child list the write extended.
	changed []string
}

// encode packs the count of created blocks, then the encodings of the created
// and the changed blocks.
func (r youngRecord) encode() []byte {
	value := binary.AppendUvarint(nil, uint64(len(r.created)))
	value = append(value, encodeChildren(r.created)...)
	return append(value, encodeChildren(r.changed)...)
}

// decodeYoungRecord decodes the value of a young log record.
func decodeYoungRecord(value []byte) (youngRecord, error) {
	// Split the blocks at the count of created blocks.
	created, n := binary.Uvarint(value)
	if n <= 0 {
		return youngRecord{}, errYoungRecord
	}
	blocks, err := decodeChildren(value[n:])
	if err != nil {
		return youngRecord{}, err
	}
	if created > uint64(len(blocks)) {
		return youngRecord{}, errYoungRecord
	}
	return youngRecord{created: blocks[:created], changed: blocks[created:]}, nil
}

// readYoungRecordTx returns the young log record seq, and false when a
// promotion deleted it.
func readYoungRecordTx(ctx context.Context, tx kvtx.TxOps, seq uint64) (youngRecord, bool, error) {
	value, found, err := tx.Get(ctx, youngRecordKey(seq))
	if err != nil || !found {
		return youngRecord{}, false, err
	}
	r, err := decodeYoungRecord(value)
	return r, err == nil, err
}

// logYoungTx appends the blocks whose child lists a write created or changed
// to the young log, whether or not a block pass runs. It reads and writes only
// the range record and the new log record.
func logYoungTx(ctx context.Context, tx kvtx.TxOps, r youngRecord) error {
	// Log nothing for a write that left every list as it was.
	if len(r.created) == 0 && len(r.changed) == 0 {
		return nil
	}
	head, err := readYoungTx(ctx, tx)
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, youngRecordKey(head.hi), r.encode()); err != nil {
		return err
	}
	head.hi++
	return writeYoungTx(ctx, tx, head)
}

// LogSettled appends the namers of the holes mark settled since the last call
// to the young log as changed blocks. The caller runs it in the transaction
// that settled them, before any write can store a hole.
//
// A settled hole has no list, so no record says its namer names it. Once a
// pass promotes, the namer is old, and a write that later stores the hole,
// such as a read-through fill or a copy's backfill, creates its list as a
// young block no young parent or owner may name. The changed record makes
// each MarkYoung seed the namer's children, so a minor pass reaches the
// stored hole and walks it. A record before a pass's bound goes at its
// promotion, but that pass reaches the namer again: it walks the hole if a
// write stored it, or settles it again and logs a new record after the bound.
func (g *RefGraph) LogSettled(ctx context.Context, mark *BlockMark) error {
	// Log nothing when no hole was settled since the last call.
	if len(mark.settledNamers) == 0 {
		return nil
	}
	namers := slices.Clone(mark.settledNamers)
	slices.Sort(namers)
	namers = slices.Compact(namers)
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		return logYoungTx(ctx, tx, youngRecord{changed: namers})
	})
	if err != nil {
		return err
	}
	mark.settledNamers = nil
	return nil
}

// YoungPosition returns the sequence of the next young log record. A major
// block pass reads it before BeginBlockPass and hands it to PromoteYoung after
// it sweeps everything it marked, so every record before it lists only blocks
// the mark saw.
func (g *RefGraph) YoungPosition(ctx context.Context) (uint64, error) {
	var pos uint64
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		head, err := readYoungTx(ctx, tx)
		pos = head.hi
		return err
	})
	return pos, err
}

// MarkYoung marks the young blocks in one snapshot. A block is young while a
// young log record lists it as created; every other tracked block is old.
// After a block pass that sweeps everything it marked and promotes its mark,
// an old block names only old blocks, as PromoteYoung describes, so a young
// block can be live only through a root, an owner that is not a block, a young
// parent, or an old parent whose list a later write changed, which the log
// lists. The mark seeds from those and walks only the young blocks' lists. A
// child outside the young blocks is checked for a list but not walked: one
// without a list counts in mark.Unknown, as in MarkBlocks, since it may yet
// arrive as a young block that an old parent names.
//
// Its unreached blocks are the young blocks the snapshot listed and the walk
// did not reach. A block pass must be open: the sweep's rescue keeps the
// blocks writes adopt after the snapshot, and grows the young scope with the
// records written since, so its walks descend into blocks created during the
// pass. The caller runs it only while YoungReady reports the young log
// complete.
func (g *RefGraph) MarkYoung(ctx context.Context) (*BlockMark, error) {
	var mark *BlockMark
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Load the young blocks, seeding those with an owner, and list the
		// changed blocks.
		head, err := readYoungTx(ctx, tx)
		if err != nil {
			return err
		}
		mark = &BlockMark{Live: NewNodeSet(), young: NewNodeSet(), youngSeen: head.hi, youngBound: head.hi}
		live, young := mark.Live, mark.young
		var stack []namedNode
		var changed []string
		for seq := head.lo; seq < head.hi; seq++ {
			r, found, err := readYoungRecordTx(ctx, tx, seq)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			created := slices.DeleteFunc(r.created, func(block string) bool { return !young.Add(block) })
			owned, err := ownedBlocksTx(ctx, tx, created)
			if err != nil {
				return err
			}
			for _, b := range owned {
				if live.Add(b.node) {
					stack = append(stack, b)
				}
			}
			changed = append(changed, r.changed...)
		}
		mark.YoungBlocks = young.Len()

		// Seed the young roots.
		roots := graphKey(kindRoot)
		err = tx.ScanPrefixKeys(ctx, roots, func(key []byte) error {
			mark.Roots++
			if root := lastNode(roots, key); young.Has(root) && live.Add(root) {
				stack = append(stack, namedNode{root, NodeGCRoot})
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Seed the children of each old changed block, which may name young
		// blocks its first list did not.
		seen := NewNodeSet()
		for _, parent := range changed {
			if young.Has(parent) || !seen.Add(parent) {
				continue
			}
			children, _, err := readChildrenTx(ctx, tx, parent)
			if err != nil {
				return err
			}
			for _, child := range children {
				if live.Add(child) {
					stack = append(stack, namedNode{child, parent})
				}
			}
		}

		// Walk the young blocks from the seeds.
		if _, err := walkTx(ctx, tx, live, stack, young, mark.noteUnknown, nil); err != nil {
			return err
		}

		// List the tracked young blocks the walk did not reach.
		listed := NewNodeSet()
		for seq := head.lo; seq < head.hi; seq++ {
			r, found, err := readYoungRecordTx(ctx, tx, seq)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			for _, block := range r.created {
				if live.Has(block) || !listed.Add(block) {
					continue
				}
				tracked, err := tx.Exists(ctx, graphKey(kindChildren, block))
				if err != nil {
					return err
				}
				if tracked {
					mark.Unreached = append(mark.Unreached, block)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mark, nil
}

// ownedBlocksTx returns the blocks that have an owner that is not a block,
// each named by its first such owner. It loads the index pages of their
// incoming records together first.
func ownedBlocksTx(ctx context.Context, tx kvtx.Tx, blocks []string) ([]namedNode, error) {
	// Load the incoming records' pages together.
	keys := make([][]byte, 0, len(blocks))
	for _, block := range blocks {
		keys = append(keys, graphKey(kindIncoming, block))
	}
	if err := kvtx.Prefetch(ctx, tx, keys); err != nil {
		return nil, err
	}

	// Find each block's first owner.
	var owned []namedNode
	for i, block := range blocks {
		owner, err := firstOwnerTx(ctx, tx, keys[i])
		if err != nil {
			return nil, err
		}
		if owner != "" {
			owned = append(owned, namedNode{block, owner})
		}
	}
	return owned, nil
}

// firstOwnerTx returns the subject of the first incoming record under prefix
// other than the unreferenced queue, or empty when there is none.
func firstOwnerTx(ctx context.Context, tx kvtx.Tx, prefix []byte) (string, error) {
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	for it.Next() {
		if subject := lastNode(prefix, it.Key()); subject != NodeUnreferenced {
			return subject, nil
		}
	}
	return "", it.Err()
}

// growYoungTx adds the blocks created in the young log records written since
// the mark last read it to the mark's young scope, so a walk descends into
// blocks created during the pass. It does nothing for a mark without a scope.
func growYoungTx(ctx context.Context, tx kvtx.Tx, mark *BlockMark) error {
	// Grow only a minor pass's scope.
	if mark.young == nil {
		return nil
	}
	head, err := readYoungTx(ctx, tx)
	if err != nil {
		return err
	}
	for seq := max(mark.youngSeen, head.lo); seq < head.hi; seq++ {
		r, found, err := readYoungRecordTx(ctx, tx, seq)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		for _, block := range r.created {
			mark.young.Add(block)
		}
	}
	mark.youngSeen = max(mark.youngSeen, head.hi)
	return nil
}

// PromoteYoung promotes the blocks of a block pass that swept everything it
// marked, in one write transaction, and reports whether it finished. It
// deletes the young log records before bound, whose created blocks the mark
// saw, so each is now reached and old or swept. It then removes from the
// records written since bound the created blocks the mark reached and walked,
// so they are old too. A block the pass settled as a hole and a write stored
// since stays young: the pass never walked its list, and LogSettled lists its
// namer as changed, so minor passes walk it. A changed block stays in each
// record it is in: its list may have changed after the walk read it. After
// promotion, an old block names only old blocks, and every later edge from
// one is in a record. stop, when set, ends the transaction between records;
// the next call continues.
//
// A promotion cut short is not safe for a minor pass: a block of a deleted
// record is old while a block it names, created in a later record, may still
// be young, and no record lists the edge. So the first call withdraws the
// YoungReady record and the call that finishes writes it again: a promotion
// cut short by a crash or an error leaves the store not minor-ready until a
// later block pass promotes in full.
func (g *RefGraph) PromoteYoung(ctx context.Context, mark *BlockMark, bound uint64, stop func() bool) (bool, error) {
	var done bool
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Fix the end of the records to rewrite on the first call.
		head, err := readYoungTx(ctx, tx)
		if err != nil {
			return err
		}
		if mark.promoteEnd == 0 {
			mark.promoteEnd, mark.promoteNext = max(head.hi, 1), bound
			if err := tx.Delete(ctx, youngReadyKey); err != nil {
				return err
			}
		}

		// Delete the records before bound. Each transaction takes at least
		// one step, so repeated calls finish however often stop reports.
		steps := 0
		yield := func() bool {
			steps++
			return steps > 1 && stop != nil && stop()
		}
		for ; head.lo < min(bound, head.hi); head.lo++ {
			if yield() {
				return writeYoungTx(ctx, tx, head)
			}
			if err := tx.Delete(ctx, youngRecordKey(head.lo)); err != nil {
				return err
			}
		}
		if err := writeYoungTx(ctx, tx, head); err != nil {
			return err
		}

		// Remove the reached created blocks from the later records.
		for ; mark.promoteNext < min(mark.promoteEnd, head.hi); mark.promoteNext++ {
			if yield() {
				return nil
			}
			key := youngRecordKey(mark.promoteNext)
			r, found, err := readYoungRecordTx(ctx, tx, mark.promoteNext)
			if err != nil || !found {
				if err != nil {
					return err
				}
				continue
			}
			kept := slices.DeleteFunc(slices.Clone(r.created), mark.walked)
			switch {
			case len(kept) == len(r.created):
			case len(kept) == 0 && len(r.changed) == 0:
				err = tx.Delete(ctx, key)
			default:
				err = tx.Set(ctx, key, youngRecord{created: kept, changed: r.changed}.encode())
			}
			if err != nil {
				return err
			}
		}
		done = true
		return tx.Set(ctx, youngReadyKey, nil)
	})
	return done, err
}

// errYoungRecord reports a young log record that does not decode.
var errYoungRecord = errors.New("malformed young log record")
