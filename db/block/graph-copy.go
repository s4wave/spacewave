package block

import (
	"context"

	"github.com/pkg/errors"
)

// DefaultGraphCopyReads is the number of source reads a graph copy keeps in
// flight when the caller sets none. A peer source serves one block per round
// trip, so the copy runs at most this many blocks per round trip: about
// 13,600 blocks a second at a 4.7 ms ping. More reads raise that ceiling and
// the source's lookup load with it.
const DefaultGraphCopyReads = 512

// GraphCopyOptions configures CopyGraph.
type GraphCopyOptions struct {
	// Reads is the number of blocks kept in flight, read in a few batches as
	// graphReader describes. Zero selects DefaultGraphCopyReads.
	Reads int
	// Known reports which refs already have a complete copy. The copy skips
	// their subtrees. It receives each block's unvisited children as a batch.
	Known func(ctx context.Context, refs []*BlockRef) ([]bool, error)
	// Complete runs after a block and every block under it are written, in
	// post-order.
	Complete func(ctx context.Context, ref *BlockRef) error
	// Visited observes each block after its read.
	Visited func(ref *BlockRef, data []byte)
	// Forget drops each finished block from the copy's memory, so the copy
	// holds only the blocks between the root and its frontier. A block reached
	// again through another parent is checked with Known again and copied
	// again when Known does not report it.
	Forget bool
	// CountLinked counts a block src serves from a linked file, one whose
	// StoredBlock is Linked, as present: the copy writes nothing for it and
	// releases its parents as if dst held it, so their subtrees complete. A
	// copy sets it within the Volume of the Device that linked the file, or
	// to a Device that leaves the file's chunks on the Device of the same
	// account that links it; any other copy needs the bytes.
	CountLinked bool
	// Missing handles a block src cannot serve now, because it does not hold
	// it or no source served it (IsNotAvailable). Returning nil leaves a hole:
	// the copy continues, and writes the hole's ancestors without completing
	// them. Without Missing, the copy returns the read error.
	Missing func(ctx context.Context, ref *BlockRef) error
}

// CopySource is implemented by a store whose reads fill a cache. A graph copy
// writes every block itself, so it reads through the returned store, which
// serves the same blocks without filling.
type CopySource interface {
	// GetCopySource returns the store a graph copy reads from.
	GetCopySource() StoreOps
}

// CopyGraph copies the block DAG under root from src to dst by following the
// outgoing refs src reports, and writes each block with its refs, so dst
// rebuilds the same ref graph. It never decodes a block. A src that is a
// CopySource is read through its copy source.
//
// Each block is written only after every block under it, in post-order, the
// order a tree write produces. A destination that stages new blocks under a
// temporary owner hands each child to its parent when the parent lands, and a
// written parent finds every child present except a hole Missing accepted.
//
// Returns ErrNotFound or ErrUnavailable for a block src cannot serve, unless
// Missing accepts it, and ErrRefsUnknown for a block src holds without its
// refs, each wrapped with the block ref. Known, Complete, Visited and Missing
// run on the calling goroutine, one at a time.
func CopyGraph(ctx context.Context, src, dst StoreOps, root *BlockRef, opts *GraphCopyOptions) error {
	// Skip an empty root and fill missing options.
	if root.GetEmpty() {
		return nil
	}
	if opts == nil {
		opts = &GraphCopyOptions{}
	}
	if cs, ok := src.(CopySource); ok {
		src = cs.GetCopySource()
	}

	// Run the copy from the source to the destination.
	c := &graphCopy{
		src:   src,
		dst:   dst,
		opts:  opts,
		nodes: make(map[string]*graphCopyNode),
	}
	return c.run(ctx, root)
}

// graphCopy holds the state of one CopyGraph call. Only the calling goroutine
// touches nodes and queue; readers report through results.
type graphCopy struct {
	src, dst StoreOps
	opts     *GraphCopyOptions

	// nodes holds every block seen so far, keyed by marshaled ref.
	nodes map[string]*graphCopyNode
	// queue holds blocks waiting for a read, popped last in first out so the
	// copy runs depth first and completes subtrees early.
	queue []*graphCopyNode
}

// graphCopyNode is one block of the copy.
type graphCopyNode struct {
	ref *BlockRef
	// data and refs hold the block read from src until it is written.
	data []byte
	refs []*BlockRef
	// waiting counts children not yet complete.
	waiting int
	// parents wait for this block to complete.
	parents []*graphCopyNode
	// done is set once the block and its subtree are complete or known.
	done bool
	// holed is set when the subtree under the block has a hole.
	holed bool
}

// run copies the graph under root. It returns only after every reader it
// started has returned.
func (c *graphCopy) run(ctx context.Context, root *BlockRef) error {
	// Stop immediately when the root already has a complete copy.
	known, err := c.known(ctx, []*BlockRef{root})
	if err != nil || known[0] {
		return err
	}

	// Copy blocks depth first until the queue drains.
	rootNode := &graphCopyNode{ref: root}
	c.nodes[root.MapKey()] = rootNode
	c.queue = append(c.queue, rootNode)
	ctx, cancel := context.WithCancel(ctx)
	reader := newGraphReader(c.src, c.opts.Reads, func(node *graphCopyNode) *BlockRef { return node.ref })
	defer func() {
		cancel()
		reader.close()
	}()
	for {
		// Keep the batch reads full, then consume one result.
		c.queue = reader.fill(ctx, c.queue)
		res, ok := reader.next()
		if !ok {
			return nil
		}
		if res.end {
			if res.err != nil {
				return res.err
			}
			continue
		}
		if res.err != nil && c.opts.Missing != nil && IsNotAvailable(res.err) {
			if err := c.opts.Missing(ctx, res.node.ref); err != nil {
				return err
			}
			if err := c.release(ctx, res.node); err != nil {
				return err
			}
			continue
		}
		if res.err != nil {
			return res.err
		}
		if c.opts.Visited != nil {
			c.opts.Visited(res.node.ref, res.stored.Data)
		}

		// Count a linked block as present, as Known counts a proved one.
		//
		// Invariant: a proof may count a block present on the strength of a
		// linked file only when no Volume tier holds that block; a Volume
		// copy is never skipped, dropped or deleted because a file serves it.
		// A copy chain reads the linked files' copy tier, and the peers it
		// lets answer linked, after every Volume tier, and a peer answers
		// linked only for a block its own linked file serves, so a Linked
		// block is one no tier of the chain holds, and the copy writes and
		// deletes nothing for it.
		if res.stored.Linked && c.opts.CountLinked {
			if err := c.settle(ctx, res.node); err != nil {
				return err
			}
			continue
		}
		if err := c.expand(ctx, res.node, res.stored.Data, res.stored.Refs); err != nil {
			return err
		}
	}
}

// expand holds a read block until its write, queues its children that are
// neither seen nor known, and completes the block when none remain.
func (c *graphCopy) expand(ctx context.Context, node *graphCopyNode, data []byte, refs []*BlockRef) error {
	// Hold the block until its subtree completes.
	node.data, node.refs = data, refs

	// Link the children into the copy graph, collecting unseen ones.
	var fresh []*graphCopyNode
	for _, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		key := ref.MapKey()
		if child := c.nodes[key]; child != nil {
			// A node expands once, so a child it already linked has it as
			// the last parent: a repeated ref costs no scan of the parents.
			if !child.done && (len(child.parents) == 0 || child.parents[len(child.parents)-1] != node) {
				child.parents = append(child.parents, node)
				node.waiting++
			}
			continue
		}
		child := &graphCopyNode{ref: ref, parents: []*graphCopyNode{node}}
		node.waiting++
		c.nodes[key] = child
		fresh = append(fresh, child)
	}

	// Skip known children and queue the rest for copying.
	if len(fresh) != 0 {
		refs := make([]*BlockRef, len(fresh))
		for i, child := range fresh {
			refs[i] = child.ref
		}
		known, err := c.known(ctx, refs)
		if err != nil {
			return err
		}
		for i, child := range fresh {
			if known[i] {
				child.done = true
				child.parents = nil
				node.waiting--
				c.forget(child)
				continue
			}
			c.queue = append(c.queue, child)
		}
	}

	// Complete the node when no child is outstanding.
	if node.waiting == 0 {
		return c.complete(ctx, node)
	}
	return nil
}

// complete writes a block whose subtree is written, marks it complete unless
// the subtree has a hole, and completes each parent that no longer waits on a
// child.
func (c *graphCopy) complete(ctx context.Context, node *graphCopyNode) error {
	// Write and complete each node, then any parent it fully released.
	stack := []*graphCopyNode{node}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		_, err := c.dst.PutBlockBatch(ctx, []*PutBatchEntry{{
			Ref:       node.ref,
			Data:      node.data,
			Refs:      node.refs,
			RefsKnown: true,
		}})
		if err != nil {
			return errors.Wrap(err, node.ref.MarshalString())
		}
		node.data, node.refs = nil, nil
		if !node.holed && c.opts.Complete != nil {
			if err := c.opts.Complete(ctx, node.ref); err != nil {
				return err
			}
		}
		stack = append(stack, c.finish(node)...)
	}
	return nil
}

// release finishes a hole and completes each parent that no longer waits on a
// child.
func (c *graphCopy) release(ctx context.Context, hole *graphCopyNode) error {
	hole.holed = true
	return c.settle(ctx, hole)
}

// settle finishes a node the copy writes nothing for and completes each parent
// that no longer waits on a child.
func (c *graphCopy) settle(ctx context.Context, node *graphCopyNode) error {
	for _, parent := range c.finish(node) {
		if err := c.complete(ctx, parent); err != nil {
			return err
		}
	}
	return nil
}

// finish marks a node done, passes a hole under it to its parents, and returns
// the parents that no longer wait on a child.
func (c *graphCopy) finish(node *graphCopyNode) []*graphCopyNode {
	// Release each parent from this child, carrying its hole up.
	node.done = true
	var ready []*graphCopyNode
	for _, parent := range node.parents {
		parent.holed = parent.holed || node.holed
		parent.waiting--
		if parent.waiting == 0 {
			ready = append(ready, parent)
		}
	}
	node.parents = nil
	c.forget(node)
	return ready
}

// forget drops a finished node from the seen set when the copy forgets.
func (c *graphCopy) forget(node *graphCopyNode) {
	if c.opts.Forget {
		delete(c.nodes, node.ref.MapKey())
	}
}

// known reports which refs already have a complete copy.
func (c *graphCopy) known(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	// Report nothing known without a Known callback.
	if c.opts.Known == nil {
		return make([]bool, len(refs)), nil
	}

	// Call the Known callback and validate its result length.
	known, err := c.opts.Known(ctx, refs)
	if err != nil {
		return nil, err
	}
	if len(known) != len(refs) {
		return nil, errors.Errorf("known returned %d results for %d refs", len(known), len(refs))
	}
	return known, nil
}
