package kvtx

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// landSegment is the most units one Land of a sliced background write takes,
// and so the most work a foreground writer that starts waiting waits behind,
// with the slice's closing moves and its commit. At the 2.1 ms per block a
// cold 14 GB SQLite World took, a segment of blocks lands in about 35 ms. A
// smaller segment costs more: each Land reads and writes the Volume's state
// record and appends one journal record, about a twentieth of what sixteen
// blocks cost.
const landSegment = 16

// slicedWrite is one write of n units, such as the blocks of a batch or the
// roots of a renewal, that writeSliced lands in order, in as many
// transactions as foreground writers make it end.
type slicedWrite struct {
	// n is the number of units.
	n int
	// land lands units [lo, hi) in p at now. When closing is set, the
	// transaction ends after them, and land also makes the moves that hold
	// every unit of the transaction, [start, hi).
	land func(ctx context.Context, p *physicalTx, now time.Time, start, lo, hi int, closing bool) error
	// committed, when set, runs after the transaction of [start, end)
	// commits.
	committed func(start, end int)
}

// writeSliced lands w through withDirect, committing fully when durable is
// set. A foreground write lands every unit in one Land and one transaction. A
// background write, one ctx marks with kvtx.WithBackgroundWriteContext, keeps
// the contract of kvtx.ForegroundQueueStore: it lands units in segments of
// landSegment, and once a foreground writer waits for the store's writer it
// closes the transaction after the next segment and commits, and the rest
// takes the writer again behind the waiter. Without waiters a background
// write is one transaction too.
//
// A committed transaction is a complete write of its units: w.land holds
// every unit when it closes, so a crash, or an error in a later transaction,
// leaves a prefix of the write stored and held, which a retry puts again as
// stored. Each segment's Land holds its blocks only through the parents the
// segment stores; a block a later segment of the same transaction names gets
// that parent as an up pair, and one no parent names gets its holder from
// the closing moves. All of them carry one clock reading, so their journal
// records age together and the collector nets them in one pass before it
// can free a block whose holder comes later in the transaction.
func (v *Volume) writeSliced(ctx context.Context, durable bool, w slicedWrite) error {
	yields := kvtx.IsBackgroundWriteContext(ctx)
	for start := 0; start < w.n; {
		end := start
		err := v.withDirect(ctx, durable, func(p *physicalTx) error {
			now := v.now()
			for closing := false; !closing; {
				hi := w.n
				if yields {
					hi = min(end+landSegment, w.n)
				}
				closing = hi == w.n || kvtx.ForegroundWaiting(v.kvtxStore)
				if err := w.land(ctx, p, now, start, end, hi, closing); err != nil {
					return err
				}
				end = hi
			}
			return nil
		})
		if err != nil {
			return err
		}
		if w.committed != nil {
			w.committed(start, end)
		}
		start = end
	}
	return nil
}

// landSliced stores entries through writeSliced and returns, for each,
// whether the store held it before. It lands them children first, so every
// segment boundary is one Land accepts: a born block's children are stored
// by its own segment or an earlier one. roots, when set, returns the moves
// that hold the entries of one transaction, and committed records them once
// it commits.
func (v *Volume) landSliced(
	ctx context.Context,
	durable bool,
	entries []*block.PutBatchEntry,
	roots func(slice []*block.PutBatchEntry) []block.NamedRoot,
	committed func(moves []block.NamedRoot),
) ([]bool, error) {
	// Land the entries children first, recording each one's answer at its
	// place in entries.
	ordered, at := childrenFirst(entries)
	out := make([]bool, len(entries))
	var moves []block.NamedRoot
	err := v.writeSliced(ctx, durable, slicedWrite{
		n: len(ordered),
		land: func(ctx context.Context, p *physicalTx, now time.Time, start, lo, hi int, closing bool) error {
			// Hold the transaction's entries with the closing segment.
			moves = nil
			if closing && roots != nil {
				moves = roots(ordered[start:hi])
			}
			existed, err := block_gc.Land(ctx, p.rec, p.blocks, now, ordered[lo:hi], block_gc.BucketRoots{Roots: moves})
			if err != nil {
				return err
			}
			for i, e := range existed {
				out[at[lo+i]] = e
			}
			return nil
		},
		committed: func(int, int) {
			if committed != nil {
				committed(moves)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// childrenFirst returns entries ordered so each entry follows the entries it
// names, keeping the given order where it already does, and the index in
// entries of each returned entry. A content-addressed batch has no cycle.
func childrenFirst(entries []*block.PutBatchEntry) ([]*block.PutBatchEntry, []int) {
	// Find each block's first entry, and keep the order when no entry names
	// a later one.
	first := make(map[string]int, len(entries))
	for i, e := range entries {
		if e != nil && !e.Ref.GetEmpty() {
			if _, ok := first[string(block_gc.BlockKey(e.Ref))]; !ok {
				first[string(block_gc.BlockKey(e.Ref))] = i
			}
		}
	}
	at := make([]int, len(entries))
	ordered := true
	for i, e := range entries {
		at[i] = i
		for _, child := range entryChildren(e) {
			if j, ok := first[string(block_gc.BlockKey(child))]; ok && j > i {
				ordered = false
			}
		}
	}
	if ordered {
		return entries, at
	}

	// Emit each entry after its children, depth first without recursion.
	out := make([]*block.PutBatchEntry, 0, len(entries))
	at = at[:0]
	done := make([]bool, len(entries))
	type frame struct{ i, next int }
	for root := range entries {
		stack := []frame{{i: root}}
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			if done[top.i] {
				stack = stack[:len(stack)-1]
				continue
			}
			children := entryChildren(entries[top.i])
			if top.next < len(children) {
				j, ok := first[string(block_gc.BlockKey(children[top.next]))]
				top.next++
				if ok && !done[j] {
					stack = append(stack, frame{i: j})
				}
				continue
			}
			done[top.i] = true
			out, at = append(out, entries[top.i]), append(at, top.i)
			stack = stack[:len(stack)-1]
		}
	}
	return out, at
}

// entryChildren returns the blocks entry names, none for a tombstone.
func entryChildren(e *block.PutBatchEntry) []*block.BlockRef {
	if e == nil || e.Tombstone {
		return nil
	}
	return e.Refs
}
