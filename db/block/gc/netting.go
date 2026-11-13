package block_gc

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// netting resolves one slice's holder changes to a fixpoint in memory and
// writes only what changed, as Collector describes. Every change to a count
// runs through it: the journal's deltas, a fill's ups and downs, and the
// releases and re-holds of the free.
//
// Each block it reaches is a node, read at most once: its count record when
// its delta is not zero when the walk reaches it, and its child list only
// when it is or reaches zero. A node whose deltas cancel before the walk
// reaches it is never read, so a shared child whose new parent's birth and
// old parent's release net in one slice costs nothing.
//
// Invariants:
//
//  1. Netting never moves a child for its parent's count: a block at zero
//     keeps its children's holders until the free releases them, so every
//     stored block but the Dying one holds each child it lists. Only the
//     free's release and re-hold move children, as freeBlock and
//     continueRelease describe.
//  2. A node's holders are its first holders plus every delta it received,
//     in any order: deltas add, so the order the walk visits nodes changes
//     only how many reads it takes, never the result. A node below zero
//     while the walk runs counts as zero; one still below zero at the end is
//     an underflow, kept with one holder.
//  3. A block the walk reads that ends at zero takes the journal's end as
//     its ripe sequence whenever it could have gained a holder in the
//     interval: it had holders first, received any up, or a put named it
//     again. A block the walk never reads keeps its records, ripe sequence
//     included: one that stayed at zero untouched, and one whose ups and
//     downs in the slice cancelled before the walk reached it, whose holders
//     end where they began.
//  4. The Dying block, the one whose paged release is in progress, takes no
//     queue entry: the free that marked it Dying ends its release, as
//     Collector.endRelease describes.
//  5. A block without a child list is never zero: its holders floor at the
//     one its absent record implies, and no release reaches past it.
type netting struct {
	// c is the collector, for its pace and writer queue.
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
	// nodes are the blocks the slice reached, by block key.
	nodes map[string]*node
	// downs and rest are the nodes to visit: those whose delta was negative
	// when queued first, so a freed parent's release reaches a shared child
	// before the child's up from its new parent is read.
	downs, rest []*node
	// moved counts the children the slice's releases and re-holds moved.
	moved int
	// moves is the slice's budget of moved children.
	moves int
	// full reports that a paged release or re-hold has children left.
	full bool
}

// node is one block a netting slice reached.
type node struct {
	// key is the block key.
	key []byte
	// ref is the block.
	ref *block.BlockRef
	// delta is the holder change received since the node was last applied.
	delta int64
	// check asks the walk to visit the node with no delta: a put named it
	// again, or an underflow kept it.
	check bool
	// queued reports that the node waits in downs or rest.
	queued bool

	// read reports that holders0 and ripe0 hold its count record.
	read bool
	// holders0 and ripe0 are its count record when the slice read it.
	holders0, ripe0 uint64
	// holders are its holders now, below zero while the walk runs when a
	// down reached it before an up.
	holders int64

	// listRead reports that listed and children hold its child list.
	listRead bool
	// listed reports that it has a child list the free may release: not an
	// unmarked one, whose children the settle's sweep releases, as settle.go
	// describes.
	listed bool
	// unmarked reports that the settle unmarked its list.
	unmarked bool
	// children are its child list.
	children []*block.BlockRef

	// raised reports that it received an up.
	raised bool
	// again reports that a put named it again in the interval.
	again bool
	// releases counts the releases that reached it.
	releases uint64
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

// node returns the node of the block at key, adding it.
func (n *netting) node(key []byte) (*node, error) {
	// Return the node already added.
	if nd, ok := n.nodes[string(key)]; ok {
		return nd, nil
	}

	// Add a node for the block at key.
	ref, _, err := ParseBlockKey(key)
	if err != nil {
		return nil, err
	}
	nd := &node{key: key, ref: ref}
	n.nodes[string(key)] = nd
	return nd, nil
}

// add adds d to the holders of the block at key and queues it.
func (n *netting) add(key []byte, d int64) error {
	// Add d to the node, noting a raise, and queue it.
	nd, err := n.node(key)
	if err != nil {
		return err
	}
	nd.delta += d
	if d > 0 {
		nd.raised = true
	}
	n.enqueue(nd)
	return nil
}

// visit queues the block at key for a visit with no delta, noting a put
// that named it again.
func (n *netting) visit(key []byte, again bool) error {
	// Mark the node for a check and queue it.
	nd, err := n.node(key)
	if err != nil {
		return err
	}
	nd.check = true
	nd.again = nd.again || again
	n.enqueue(nd)
	return nil
}

// enqueue queues nd for the walk unless it waits already.
func (n *netting) enqueue(nd *node) {
	if nd.queued {
		return
	}
	nd.queued = true
	if nd.delta < 0 {
		n.downs = append(n.downs, nd)
	} else {
		n.rest = append(n.rest, nd)
	}
}

// resolve walks the queued nodes to a fixpoint, then keeps each node still
// below zero with one holder, as an underflow, and walks again.
func (n *netting) resolve(ctx context.Context) error {
	for {
		// Walk to the fixpoint, the falling nodes first.
		for len(n.downs) != 0 || len(n.rest) != 0 {
			var nd *node
			if len(n.downs) != 0 {
				nd, n.downs = n.downs[0], n.downs[1:]
			} else {
				nd, n.rest = n.rest[0], n.rest[1:]
			}
			nd.queued = false
			if err := n.apply(ctx, nd); err != nil {
				return err
			}
		}

		// Keep a node below zero with one holder: a holder vanished that
		// the journal never added.
		under := false
		for _, nd := range n.nodes {
			if !nd.read || nd.holders >= 0 {
				continue
			}
			n.res.Underflows++
			nd.holders, nd.check, under = 1, true, true
			n.enqueue(nd)
		}
		if !under {
			return nil
		}
	}
}

// apply applies nd's delta, and reads the child list of a block that is or
// reaches zero, which decides its queue entry.
func (n *netting) apply(ctx context.Context, nd *node) error {
	// Skip a node whose deltas cancelled, unless it asks for a visit.
	if nd.delta == 0 && !nd.check {
		return nil
	}
	nd.check = false

	// Read its count record once, and apply its delta.
	if !nd.read {
		holders, ripe, err := n.rec.GetHolders(ctx, nd.ref)
		if err != nil {
			return err
		}
		nd.read, nd.holders0, nd.ripe0 = true, holders, ripe
		nd.holders = int64(holders) //nolint:gosec // counts stay far below the int64 range.
	}
	nd.holders += nd.delta
	nd.delta = 0

	// A node that had holders and keeps them has no queue entry to move.
	if nd.holders > 0 && nd.holders0 != 0 {
		return nil
	}
	return n.readList(ctx, nd)
}

// readList reads nd's child list once. While the generation is Sweeping it
// also reads the block's run entry, and an unmarked block counts as
// unlisted: its holders floor at its implied one, so it never queues and the
// free never releases it, since the sweep releases each child outside the
// run its list holds and a free would release them twice.
func (n *netting) readList(ctx context.Context, nd *node) error {
	// Read the list unless read already.
	if nd.listRead {
		return nil
	}
	children, listed, err := n.rec.GetChildren(ctx, nd.ref)
	if err != nil {
		return err
	}
	if listed {
		if nd.unmarked, err = n.rec.Unmarked(ctx, nd.key); err != nil {
			return err
		}
	}
	nd.listRead, nd.listed, nd.children = true, listed && !nd.unmarked, children
	return nil
}

// move moves the child at key by dir for its parent's release or re-hold,
// counting a release.
func (n *netting) move(key []byte, dir int64) error {
	if err := n.add(key, dir); err != nil {
		return err
	}
	if dir < 0 {
		n.nodes[string(key)].releases++
	}
	return nil
}

// continueRelease moves the children of the paged release or re-hold in
// progress from its release pages, within the slice's moves and stopping
// between pages once a writer waits, and moves its Next past them. It
// deletes each page once its last child moves. A re-hold ends with its last
// child; a release stays in the state with Next at its end until the free
// ends it, deleting the Dying block or starting its re-hold, as
// Collector.endRelease describes. The slice writes the state.
//
// Invariants, which together keep every child's count exact:
//
//  1. Each child of the block moves exactly once: the pages hold the
//     children not moved yet from the transaction that starts the release
//     or re-hold, and Next moves in the transaction that moves the children
//     before it, so a crash or restart resumes at the first child not
//     moved, never moving one twice or skipping one.
//  2. A child of a release counts its parent until it moves, and a child of
//     a re-hold does not until it moves; the audit counts the unmoved part
//     so. The block's bytes and list stay stored until the release ends.
//  3. At most one paged release or re-hold is in progress, and no free
//     starts while one is in the state.
//  4. A page missing before its children move is an error, naming the
//     block, that stops every pass until a repair, so a damaged release
//     keeps every child.
func (n *netting) continueRelease(ctx context.Context) error {
	for r := n.state.GetRelease(); r != nil && r.GetNext() < r.GetChildren(); r = n.state.GetRelease() {
		// Let a waiting writer in between pages, and stop at the budget.
		if n.moved == n.moves || (n.moved != 0 && n.c.waiting()) {
			n.full = true
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
		dir := int64(-1)
		if r.GetRehold() {
			dir = 1
		}
		end := min(uint64(len(children)), within+uint64(n.moves-n.moved)) //nolint:gosec // the budget is small.
		for _, child := range children[within:end] {
			if err := n.move(BlockKey(child), dir); err != nil {
				return err
			}
		}
		n.moved += int(end - within) //nolint:gosec // at most the budget.
		r.Next += end - within

		// Delete a page once moved, and end a re-hold after the last.
		if end == uint64(len(children)) {
			if err := n.rec.deleteReleasePage(ctx, page); err != nil {
				return err
			}
		}
		if r.GetRehold() && r.GetNext() >= r.GetChildren() {
			n.state.Release = nil
		}
	}
	return nil
}

// applyFillUps applies the up pairs of the fill records not applied yet,
// whatever their age, within budget entries, a first record whole however
// large, and deletes their records. It reports whether more remain.
func (n *netting) applyFillUps(ctx context.Context, budget int) (int, bool, error) {
	used, more := 0, false
	err := n.rec.scanFillUps(ctx, func(key []byte, children []*block.BlockRef) (bool, error) {
		if used != 0 && used+len(children) > budget {
			more = true
			return false, nil
		}
		for _, child := range children {
			if err := n.add(BlockKey(child), 1); err != nil {
				return false, err
			}
		}
		used += len(children)
		return true, n.rec.tx.Delete(ctx, key)
	})
	return used, more, err
}

// write writes each node whose records changed: its count record and its
// queue entry.
func (n *netting) write(ctx context.Context) error {
	for _, nd := range n.nodes {
		if err := n.writeNode(ctx, nd); err != nil {
			return err
		}
	}
	return nil
}

// writeNode writes nd's records that changed.
func (n *netting) writeNode(ctx context.Context, nd *node) error {
	// A node never read changed nothing.
	if !nd.read {
		return nil
	}

	// Floor a block without a child list at its implied holder. A release
	// that reached one leaked: its own children keep its holder, garbage the
	// audit counts. An unmarked block is no leak, since the sweep frees it.
	listed := nd.listRead && nd.listed
	if nd.listRead && !nd.listed && !nd.unmarked {
		n.res.Leaked += nd.releases
	}
	holders := uint64(max(nd.holders, 0)) //nolint:gosec // not negative.
	if holders == 0 && !listed {
		holders = 1
	}

	// Ripen a block at zero that could have gained a holder: a child a free
	// released takes the journal's end, after every record a Land that held
	// it through its parent could have written.
	var ripe uint64
	if holders == 0 {
		ripe = nd.ripe0
		if nd.holders0 != 0 || nd.raised || nd.again {
			ripe = n.state.GetNextSeq()
		}
	}

	// Drop the proof of a block that lost its last holder: a proof counts
	// only for a held block, as a pending one does.
	if holders == 0 && nd.holders0 != 0 {
		if err := n.rec.DeleteProof(ctx, nd.ref); err != nil {
			return err
		}
	}

	// Write the count record when it changed.
	if holders != nd.holders0 || ripe != nd.ripe0 {
		n.res.Counted++
		if err := n.rec.SetHolders(ctx, nd.ref, holders, ripe); err != nil {
			return err
		}
	}

	// Move its queue entry when it turned Dead, stopped being Dead or
	// ripened again. A node whose list was never read had holders and kept
	// them. The Dying block has none: the free took it off the queue.
	if !nd.listRead || n.dying(nd.key) {
		return nil
	}
	dead0 := nd.holders0 == 0 && nd.listed
	dead := holders == 0 && listed
	if dead && !dead0 {
		n.res.Queued++
	}
	if dead0 && (!dead || ripe != nd.ripe0) {
		if err := n.rec.tx.Delete(ctx, n.rec.queueKey(nd.ripe0, nd.key)); err != nil {
			return err
		}
	}
	if dead && (!dead0 || ripe != nd.ripe0) {
		if err := n.rec.tx.Set(ctx, n.rec.queueKey(ripe, nd.key), nil); err != nil {
			return err
		}
	}
	return nil
}

// dying reports whether key is the Dying block: the one whose paged release
// is in progress or waits for the free to end it.
func (n *netting) dying(key []byte) bool {
	r := n.state.GetRelease()
	return r != nil && !r.GetRehold() && bytes.Equal(r.GetBlock(), key)
}
