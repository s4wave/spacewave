package block_gc

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// netting resolves one slice's holder changes to a fixpoint in memory and
// writes only what changed, as Collector describes. Every change to a count
// runs through it: the journal's deltas, a fill's ups and downs, and the
// cascades they start.
//
// Each block it reaches is a node, read at most once: its count record when
// its delta is not zero when the walk reaches it, its cascade record only
// when the block is or reaches zero, and its child list only when it is or
// reaches zero or cascades. A node whose deltas cancel before the walk
// reaches it is never read, so a shared child whose new parent's birth and
// old parent's release net in one slice costs nothing.
//
// Invariants:
//
//  1. A block's children count it exactly when its rel flag is clear: rel is
//     (holders == 0) != (a cascade record exists). A cascade flips rel and
//     moves each child by one in the same transaction, inline or through the
//     release pages, so the flag and the children's counts never disagree
//     across a commit.
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
//  4. At most one paged cascade is in progress, and while one is, no other
//     cascade runs: a block that crosses zero meanwhile keeps its cascade
//     record for a later slice.
//  5. A block without a child list is never zero: its holders floor at the
//     one its absent record implies, and no cascade reaches past it. A down
//     cascade due on one is dropped, its children keeping its holder.
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
	// when queued first, so a dying parent's release reaches a shared child
	// before the child's up from its new parent is read.
	downs, rest []*node
	// cascades are the cascade records the slice loaded, by block key.
	cascades map[string]byte
	// cascadesKnown is set once cascades holds every cascade record, so a
	// node's is read from it rather than the store.
	cascadesKnown bool
	// moved counts the children the slice's cascades moved.
	moved int
	// moves is the slice's budget of moved children.
	moves int
	// full reports that a cascade did not fit the slice.
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
	// check asks the walk to visit the node with no delta: it has a cascade
	// record or a put named it again.
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
	// listed reports that it has a child list the netting may cascade
	// through: not an unmarked one, whose children the settle's sweep
	// releases, as settle.go describes.
	listed bool
	// unmarked reports that the settle unmarked its list.
	unmarked bool
	// children are its child list.
	children []*block.BlockRef

	// relRead reports that rel0 and dir0 hold its first rel flag and cascade
	// record.
	relRead bool
	// rel0 and rel are its rel flag first and now: set when its children do
	// not count it.
	rel0, rel bool
	// dir0 is the direction of its cascade record as the slice began, or
	// zero when none was due.
	dir0 byte

	// raised reports that it received an up.
	raised bool
	// again reports that a put named it again in the interval.
	again bool
	// cascadeDowns counts the down cascades that reached it.
	cascadeDowns uint64
}

// newNetting starts a netting slice with a budget of moves moved children.
func (c *Collector) newNetting(rec *Records, state *LivenessState, res *CollectResult, moves int) (*netting, error) {
	start, err := state.MarshalVT()
	if err != nil {
		return nil, err
	}
	return &netting{
		c:        c,
		rec:      rec,
		state:    state,
		start:    start,
		res:      res,
		nodes:    make(map[string]*node),
		cascades: make(map[string]byte),
		moves:    moves,
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

// loadCascades loads up to limit cascade records into the slice, queueing
// each block, and reports whether more remain. Once none remains every
// cascade record is known to the slice.
//
// It reads nothing when the state counts no record. Otherwise it scans from
// where the collector's last load stopped, then from the first record up to
// there, so draining a long run of cascades reads each record once rather
// than walking the deleted ones again each slice. Where it starts changes
// only the order the slice reaches blocks, which invariant 2 says changes
// no result.
func (n *netting) loadCascades(ctx context.Context, limit int) (bool, error) {
	// Load nothing when none is due.
	due := n.state.GetCascades()
	if due == 0 {
		n.cascadesKnown = true
		return false, nil
	}

	// Load from where the last load stopped, then wrap to the first.
	from, full := n.c.cascadeFrom, false
	var last []byte
	load := func(key []byte, dir byte) (bool, error) {
		if len(n.cascades) == limit {
			full = true
			return false, nil
		}
		n.cascades[string(key)], last = dir, key
		return true, n.visit(key, false)
	}
	err := n.rec.scanCascades(ctx, from, load)
	if err == nil && !full && from != nil {
		err = n.rec.scanCascades(ctx, nil, func(key []byte, dir byte) (bool, error) {
			if bytes.Compare(key, from) >= 0 {
				return false, nil
			}
			return load(key, dir)
		})
	}
	if err != nil {
		return false, err
	}
	if last != nil {
		n.c.cascadeFrom = append(last, 0)
	}

	// Every record is loaded once the count is reached.
	n.cascadesKnown = uint64(len(n.cascades)) == due
	if !full && !n.cascadesKnown {
		return false, errors.Errorf("liveness cascade count %d is above its %d records", due, len(n.cascades))
	}
	return full, nil
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
		// the journal never added. A Dead one is Holding from here, and its
		// re-hold gives its children their holder back.
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

// apply applies nd's delta and runs its cascade when one is due.
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

	// A node that had holders and keeps them changed nothing a cascade
	// depends on, unless its cascade record is due.
	_, due := n.cascades[string(nd.key)]
	if nd.holders > 0 && nd.holders0 != 0 && !nd.relRead && !due {
		return nil
	}

	// Read its child list and rel flag; a block without a list never
	// cascades, and the write drops a down cascade due on one.
	if err := n.readList(ctx, nd); err != nil {
		return err
	}
	if err := n.readRel(ctx, nd); err != nil || !nd.listed {
		return err
	}
	if (nd.holders <= 0) == nd.rel {
		return nil
	}
	return n.cascade(ctx, nd)
}

// readList reads nd's child list once. While the generation is Sweeping it
// also reads the block's run entry, and an unmarked block counts as
// unlisted: its holders floor at its implied one, so it never queues, and
// no cascade runs through it, since the sweep releases each child outside
// the run its list holds and a cascade would release them twice.
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

// readRel reads nd's first rel flag once, from the slice's cascade records
// when it holds them all, and otherwise from the store.
func (n *netting) readRel(ctx context.Context, nd *node) error {
	// Read the flag unless read already.
	if nd.relRead {
		return nil
	}

	// Find whether a cascade is due, then derive the flag from it.
	dir, due := n.cascades[string(nd.key)]
	if !due && !n.cascadesKnown {
		var err error
		dir, due, err = n.rec.getCascade(ctx, nd.key)
		if err != nil {
			return err
		}
	}
	nd.relRead, nd.dir0 = true, dir
	nd.rel0 = (nd.holders0 == 0) != due
	nd.rel = nd.rel0
	return nil
}

// cascade moves each child of nd, whose holders crossed zero, by one in the
// direction it crossed: inline when its list fits the slice's moves, through
// release pages when the slice has moved nothing yet, and otherwise not this
// slice, leaving its cascade record due.
func (n *netting) cascade(ctx context.Context, nd *node) error {
	// Leave the cascade due while a paged one is in progress or the list
	// does not fit a slice that moved children already.
	inline := len(nd.children) <= n.moves-n.moved
	if n.state.GetRelease() != nil || (!inline && n.moved != 0) {
		n.full = true
		return nil
	}

	// Move the children of a list that fits. A release first drops the
	// block's completion proof: its children may be freed once they settle,
	// before the block itself, so the proof no longer holds. A leaf's proof
	// stays true until the free deletes it.
	dir := int64(1)
	if nd.holders <= 0 {
		dir = -1
		if len(nd.children) != 0 {
			if err := n.rec.DeleteProof(ctx, nd.ref); err != nil {
				return err
			}
		}
	}
	nd.rel = !nd.rel
	if inline {
		for _, child := range nd.children {
			if err := n.move(BlockKey(child), dir); err != nil {
				return err
			}
		}
		n.moved += len(nd.children)
		return nil
	}

	// Page a longer list and start moving it.
	if err := n.rec.putReleasePages(ctx, nd.children); err != nil {
		return err
	}
	n.state.Release = &Release{Block: nd.key, Children: uint64(len(nd.children)), Rehold: dir > 0}
	return n.continueRelease(ctx)
}

// move moves the child at key by dir for its parent's cascade.
func (n *netting) move(key []byte, dir int64) error {
	if err := n.add(key, dir); err != nil {
		return err
	}
	if dir < 0 {
		n.nodes[string(key)].cascadeDowns++
	}
	return nil
}

// continueRelease moves the children of the paged cascade in progress from
// its release pages, within the slice's moves and stopping between pages
// once a writer waits, and moves its Next past them; once the last child
// moves it clears the state's release. It deletes each page once its last
// child moves. The slice writes the state.
//
// Invariants, which together keep every child's count exact:
//
//  1. Each child of the cascade's block moves exactly once: the pages hold
//     the children not moved yet from the slice that starts the cascade,
//     which flips the block's rel flag, and Next moves in the slice that
//     moves the children before it, so a crash or restart resumes at the
//     first child not moved, never moving one twice or skipping one.
//  2. A child of a release counts its parent until it moves, and a child of
//     a re-hold does not until it moves; the audit counts the unmoved part
//     so. Either only delays a free: a released child's count runs high,
//     and nothing frees while the cascade is in progress.
//  3. At most one paged cascade is in progress, and no other cascade runs
//     until it finishes.
//  4. A page missing before its children move is an error, naming the
//     block, that stops every pass until a repair, so a damaged cascade
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

		// Delete a page once moved, and end the cascade after the last.
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

// write writes each node whose records changed: its count record, its
// cascade record and its queue entry.
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

	// Floor a block without a child list at its implied holder, and drop a
	// down cascade due on one, which can never run and would keep every free
	// waiting: its children keep the block's holder, garbage the audit counts
	// in Leaked. An up cascade stays due, keeping the frees waiting, since
	// losing a Holding block is outside the contract: a refill of the hole
	// writes its list back, and the re-hold then gives its children the
	// holder they lack. An unmarked block counts as no leak, since the sweep
	// frees it.
	listed := nd.listRead && nd.listed
	if nd.listRead && !nd.listed {
		if !nd.unmarked {
			n.res.Leaked += nd.cascadeDowns
		}
		if nd.dir0 == cascadeDown {
			if !nd.unmarked {
				n.res.Stranded++
			}
			if err := n.rec.deleteCascade(ctx, n.state, nd.key); err != nil {
				return err
			}
		}
	}
	holders := uint64(max(nd.holders, 0)) //nolint:gosec // not negative.
	if holders == 0 && !listed {
		holders = 1
	}

	// Ripen a block at zero that could have gained a holder.
	var ripe uint64
	if holders == 0 {
		ripe = nd.ripe0
		if nd.holders0 != 0 || nd.raised || nd.again {
			ripe = n.state.GetNextSeq()
		}
		if nd.holders0 != 0 {
			n.res.Queued++
		}
	}

	// Write the count record when it changed.
	if holders != nd.holders0 || ripe != nd.ripe0 {
		n.res.Counted++
		if err := n.rec.SetHolders(ctx, nd.ref, holders, ripe); err != nil {
			return err
		}
	}

	// A node whose rel flag was never read kept it and its holders: its
	// cascade and queue records stand.
	if !listed || !nd.relRead {
		return nil
	}

	// Write its cascade record when its due cascade changed.
	due0 := (nd.holders0 == 0) != nd.rel0
	due := (holders == 0) != nd.rel
	dir := cascadeUp
	if holders == 0 {
		dir = cascadeDown
	}
	switch {
	case due && !due0:
		if err := n.rec.addCascade(ctx, n.state, nd.key, dir); err != nil {
			return err
		}
	case due && (nd.holders0 == 0) != (holders == 0):
		if err := n.rec.tx.Set(ctx, n.rec.cascadeKey(nd.key), []byte{dir}); err != nil {
			return err
		}
	case !due && due0:
		if err := n.rec.deleteCascade(ctx, n.state, nd.key); err != nil {
			return err
		}
	}

	// Move its queue entry when it turned Dead, stopped being Dead or
	// ripened again.
	dead0 := nd.holders0 == 0 && nd.rel0
	dead := holders == 0 && nd.rel
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
