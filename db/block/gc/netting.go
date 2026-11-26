package block_gc

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// netting sums one transaction's holder changes per block and writes only
// what changed, as Collector describes. Every change to a count runs through
// it: the ups and downs of the logs, and the downs a free gives the children
// of the block it frees.
//
// Each block it reaches is a node holding the sum of its deltas. The write
// reads the count records of the nodes in one batch, and a node's child list
// only when it ends at zero. No block releases its children before it is
// freed, so one change never starts another, and the sum needs no walk.
//
// Invariants:
//
//  1. A node's holders are its first holders plus every delta it received,
//     in any order. One that would end below zero is an underflow: a holder
//     vanished that the logs never added, so it is kept with one holder.
//  2. A block that ends at zero takes the up log's end, UpSeq, as its ripe
//     sequence when it had holders first or received an up: every up
//     written before then has a lower sequence, so the free, which waits
//     for the up cursor to pass the ripe sequence, frees it only after they
//     net. A node that received an up is written even when its deltas
//     cancel, since the Land that wrote the up dequeued it at zero, and only
//     this write queues it again.
//  3. A block without a child list is never zero: its holders floor at the
//     one its absent record implies. While Sweeping an unmarked block counts
//     as one without a list, since the sweep deletes it.
//  4. A listed block at zero is on the free queue at its ripe sequence,
//     except from a Land that holds it, which dequeues it, to the net of
//     that Land's up, which queues it again or lifts it off zero.
//  5. The only release is a free's: each child of a freed block moves down
//     once, inline when the list fits the slice's moves, and otherwise
//     through release pages, as continueRelease describes.
type netting struct {
	// c is the collector, for its writer queue.
	c *Collector
	// rec are the slice's liveness records.
	rec *Records
	// state is the liveness state the slice writes back.
	state *LivenessState
	// start is the state as the slice began, so an unchanged one is not
	// written.
	start []byte
	// res is the pass's accounting.
	res *CollectResult
	// nodes are the blocks the slice reached, by block key, and order the
	// same nodes in the order they were reached.
	nodes map[string]*node
	order []*node
	// moved counts the children the slice's releases moved, and moves is its
	// budget.
	moved, moves int
}

// node is one block a netting slice reached.
type node struct {
	// key is the block key.
	key []byte
	// ref is the block.
	ref *block.BlockRef
	// delta is the sum of the holder changes it received.
	delta int64
	// raised reports that it received an up.
	raised bool
}

// newNetting starts a netting slice with a budget of moves moved children.
func (c *Collector) newNetting(rec *Records, state *LivenessState, res *CollectResult, moves int) (*netting, error) {
	start, err := state.MarshalVT()
	if err != nil {
		return nil, err
	}
	return &netting{
		c:     c,
		rec:   rec,
		state: state,
		start: start,
		res:   res,
		nodes: make(map[string]*node),
		moves: moves,
	}, nil
}

// add adds d to the holders of the block at key, which it copies.
func (n *netting) add(key []byte, d int64) error {
	// Find the block's node, or add one in the order reached.
	nd, ok := n.nodes[string(key)]
	if !ok {
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return err
		}
		nd = &node{key: bytes.Clone(key), ref: ref}
		n.nodes[string(key)] = nd
		n.order = append(n.order, nd)
	}
	nd.delta += d
	nd.raised = nd.raised || d > 0
	return nil
}

// release moves each of children, the child list of the freed block at key,
// down by one: inline when they fit the slice's moves, and otherwise through
// release pages, starting the paged release. A caller starts a paged one
// only while none is in progress and the slice has moved nothing yet.
func (n *netting) release(ctx context.Context, key []byte, children []*block.BlockRef) error {
	// Move a list that fits.
	if len(children) <= n.moves-n.moved {
		for _, child := range children {
			if err := n.add(BlockKey(child), -1); err != nil {
				return err
			}
		}
		n.moved += len(children)
		return nil
	}

	// Page a longer list and start moving it.
	if err := n.rec.putReleasePages(ctx, children); err != nil {
		return err
	}
	n.state.Release = &Release{Block: key, Children: uint64(len(children))}
	return n.continueRelease(ctx)
}

// continueRelease moves the children of the paged release in progress down
// from its release pages, within the slice's moves and stopping between
// pages once a writer waits, and moves its Next past them; once the last
// child moves it clears the state's release. It deletes each page once its
// last child moves. The slice writes the state.
//
// Invariants, which together keep every child's count exact:
//
//  1. Each child of the freed block moves exactly once: the pages hold the
//     children not moved yet from the free that starts the release, and Next
//     moves in the slice that moves the children before it, so a crash or
//     restart resumes at the first child not moved, never moving one twice
//     or skipping one.
//  2. A child counts the freed block until it moves; the audit counts the
//     unmoved part so. That only delays a free: a child's count runs high,
//     and nothing frees while the release is in progress.
//  3. At most one paged release is in progress.
//  4. A page missing before its children move is an error, naming the
//     block, that stops every pass until a repair, so a damaged release
//     keeps every child.
func (n *netting) continueRelease(ctx context.Context) error {
	for r := n.state.GetRelease(); r != nil && n.moved < n.moves; r = n.state.GetRelease() {
		// Let a waiting writer in between pages.
		if n.moved != 0 && n.c.waiting() {
			return nil
		}

		// Read the page holding the next child.
		page := r.GetNext() / releasePageChildren
		children, err := n.rec.getReleasePage(ctx, page)
		if err != nil {
			return err
		}
		within := r.GetNext() % releasePageChildren
		if within >= uint64(len(children)) {
			return errReleasePageMissing(r, page)
		}

		// Move the next children on it, within the moves.
		end := min(uint64(len(children)), within+uint64(n.moves-n.moved)) //nolint:gosec // the budget is small.
		for _, child := range children[within:end] {
			if err := n.add(BlockKey(child), -1); err != nil {
				return err
			}
		}
		n.moved += int(end - within) //nolint:gosec // at most the budget.
		r.Next += end - within

		// Delete a page once moved, and end the release after the last.
		if end == uint64(len(children)) {
			if err := n.rec.deleteReleasePage(ctx, page); err != nil {
				return err
			}
		}
		if r.GetNext() >= r.GetChildren() {
			n.state.Release = nil
		}
	}
	return nil
}

// write writes the records of each node that changed, its count record and
// its queue entry, then the state when it changed. It reads the nodes' count
// records in one batch.
func (n *netting) write(ctx context.Context) error {
	// Gather the nodes that may have changed, and read their counts.
	var nodes []*node
	for _, nd := range n.order {
		if nd.delta != 0 || nd.raised {
			nodes = append(nodes, nd)
		}
	}

	// While Sweeping an unmarked block's holders are in its
	// kindUnmarkedCount record, as countKind routes it, at one more read of
	// its run entry; outside Sweeping every node's is its kindCount record.
	sweeping := n.state.GetGeneration() == Generation_GENERATION_SWEEPING
	reads := make([][]byte, len(nodes))
	for i, nd := range nodes {
		kind := kindCount
		if sweeping {
			var err error
			if kind, err = n.rec.countKind(ctx, nd.key); err != nil {
				return err
			}
		}
		reads[i] = n.rec.key(kind, nd.key)
	}
	var values [][]byte
	var found []bool
	if len(reads) != 0 {
		var err error
		if values, found, err = kvtx.GetBatch(ctx, n.rec.tx, reads); err != nil {
			return err
		}
	}

	// Write each.
	for i, nd := range nodes {
		holders0, ripe0 := uint64(1), uint64(0)
		if found[i] {
			var err error
			if holders0, ripe0, err = decodeHolders(values[i]); err != nil {
				return err
			}
		}
		if err := n.writeNode(ctx, nd, holders0, ripe0); err != nil {
			return err
		}
	}

	// Write the state when it changed.
	end, err := n.state.MarshalVT()
	if err != nil || bytes.Equal(end, n.start) {
		return err
	}
	return n.rec.SetState(ctx, n.state)
}

// writeNode writes the records of nd, whose count record held holders0 and
// ripe0, that its delta changes.
func (n *netting) writeNode(ctx context.Context, nd *node, holders0, ripe0 uint64) error {
	// Apply the delta, keeping an underflow with one holder.
	sum := int64(holders0) + nd.delta //nolint:gosec // counts stay far below the int64 range.
	if sum < 0 {
		n.res.Underflows++
		sum = 1
	}
	holders := uint64(sum)

	// Floor a block without a child list at its implied holder: a hole or a
	// block not stored yet keeps its children, garbage the audit counts, and
	// an unmarked one is the sweep's.
	if holders == 0 {
		listed, unmarked, err := n.listed(ctx, nd.key)
		if err != nil {
			return err
		}
		if !listed {
			holders = 1
			if !unmarked {
				n.res.Leaked++
			}
		}
	}

	// Ripen a block at zero that could have gained a holder.
	var ripe uint64
	if holders == 0 {
		ripe = ripe0
		if holders0 != 0 || nd.raised {
			ripe = n.state.GetUpSeq()
		}
		if holders0 != 0 {
			n.res.Queued++
		}
	}

	// A block that leaves zero while Open leaves the park and the run the
	// park put it in, as Records.unpark describes.
	if holders0 == 0 && holders != 0 && n.state.GetGeneration() == Generation_GENERATION_OPEN {
		if err := n.rec.unpark(ctx, n.state, nd.key, ripe0); err != nil {
			return err
		}
	}

	// Write the count record when it changed.
	if holders != holders0 || ripe != ripe0 {
		n.res.Counted++
		if err := n.rec.SetHolders(ctx, nd.ref, holders, ripe); err != nil {
			return err
		}
	}

	// Move the queue entry: off once the block leaves zero or ripens again,
	// and on once it reaches zero or receives an up there, since the Land
	// that wrote the up dequeued it.
	if holders0 == 0 && (holders != 0 || ripe != ripe0) {
		if err := n.rec.tx.Delete(ctx, n.rec.queueKey(ripe0, nd.key)); err != nil {
			return err
		}
	}
	if holders == 0 && (holders0 != 0 || nd.raised) {
		return n.rec.tx.Set(ctx, n.rec.queueKey(ripe, nd.key), nil)
	}
	return nil
}

// listed reports whether the block at key has a child list the netting may
// queue it by, and whether the settle unmarked it: while Sweeping an
// unmarked block counts as unlisted.
func (n *netting) listed(ctx context.Context, key []byte) (listed, unmarked bool, err error) {
	listed, err = n.rec.tx.Exists(ctx, n.rec.key(kindChildren, key))
	if err != nil || !listed || n.state.GetGeneration() != Generation_GENERATION_SWEEPING {
		return listed, false, err
	}
	unmarked, err = n.rec.Unmarked(ctx, key)
	return !unmarked, unmarked, err
}
