package block

import (
	"context"
	"slices"

	"github.com/pkg/errors"
)

// DefaultGraphCopyReads is the number of source reads a graph copy keeps in
// flight when the caller sets none.
const DefaultGraphCopyReads = 16

// GraphCopyOptions configures CopyGraph.
type GraphCopyOptions struct {
	// Reads is the number of source reads kept in flight. Zero selects
	// DefaultGraphCopyReads.
	Reads int
	// Known reports which refs already have a complete copy. The copy skips
	// their subtrees. It receives each block's unvisited children as a batch.
	Known func(ctx context.Context, refs []*BlockRef) ([]bool, error)
	// Complete runs after a block and every block under it are written, in
	// post-order.
	Complete func(ctx context.Context, ref *BlockRef) error
	// Visited observes each block after its write.
	Visited func(ref *BlockRef, data []byte)
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
// Returns ErrNotFound for a missing block and ErrRefsUnknown for a block src
// holds without its refs, each wrapped with the block ref. Known, Complete
// and Visited run on the calling goroutine, one at a time.
func CopyGraph(ctx context.Context, src, dst StoreOps, root *BlockRef, opts *GraphCopyOptions) error {
	if root.GetEmpty() {
		return nil
	}
	if opts == nil {
		opts = &GraphCopyOptions{}
	}
	if cs, ok := src.(CopySource); ok {
		src = cs.GetCopySource()
	}
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
	// waiting counts children not yet complete.
	waiting int
	// parents wait for this block to complete.
	parents []*graphCopyNode
	// done is set once the block and its subtree are complete or known.
	done bool
}

// graphCopyRead is the result of reading and writing one block.
type graphCopyRead struct {
	node *graphCopyNode
	data []byte
	refs []*BlockRef
	err  error
}

// run copies the graph under root. It returns only after every reader it
// started has returned.
func (c *graphCopy) run(ctx context.Context, root *BlockRef) error {
	known, err := c.known(ctx, []*BlockRef{root})
	if err != nil || known[0] {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	reads := c.opts.Reads
	if reads <= 0 {
		reads = DefaultGraphCopyReads
	}
	results := make(chan graphCopyRead, reads)
	inFlight := 0
	defer func() {
		cancel()
		for ; inFlight != 0; inFlight-- {
			<-results
		}
	}()

	rootNode := &graphCopyNode{ref: root}
	c.nodes[root.MarshalString()] = rootNode
	c.queue = append(c.queue, rootNode)
	for {
		for inFlight < reads && len(c.queue) != 0 {
			node := c.queue[len(c.queue)-1]
			c.queue = c.queue[:len(c.queue)-1]
			inFlight++
			go func() {
				results <- c.copyBlock(ctx, node)
			}()
		}
		if inFlight == 0 {
			return nil
		}
		res := <-results
		inFlight--
		if res.err != nil {
			return res.err
		}
		if c.opts.Visited != nil {
			c.opts.Visited(res.node.ref, res.data)
		}
		if err := c.expand(ctx, res.node, res.refs); err != nil {
			return err
		}
	}
}

// copyBlock reads one block with its refs from src and writes both to dst.
func (c *graphCopy) copyBlock(ctx context.Context, node *graphCopyNode) graphCopyRead {
	stored, err := c.src.GetStoredBlock(ctx, node.ref)
	if err == nil && stored == nil {
		err = ErrNotFound
	}
	if err == nil && !stored.RefsKnown {
		err = ErrRefsUnknown
	}
	if err == nil {
		err = c.dst.PutBlockBatch(ctx, []*PutBatchEntry{{
			Ref:  node.ref,
			Data: stored.Data,
			Refs: stored.Refs,
		}})
	}
	if err != nil {
		return graphCopyRead{err: errors.Wrap(err, node.ref.MarshalString())}
	}
	return graphCopyRead{node: node, data: stored.Data, refs: stored.Refs}
}

// expand queues the children of a written block that are neither seen nor
// known, and completes the block when none remain.
func (c *graphCopy) expand(ctx context.Context, node *graphCopyNode, refs []*BlockRef) error {
	var fresh []*graphCopyNode
	for _, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		key := ref.MarshalString()
		if child := c.nodes[key]; child != nil {
			if !child.done && !slices.Contains(child.parents, node) {
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
				continue
			}
			c.queue = append(c.queue, child)
		}
	}
	if node.waiting == 0 {
		return c.complete(ctx, node)
	}
	return nil
}

// complete marks a block complete and completes each parent that no longer
// waits on a child.
func (c *graphCopy) complete(ctx context.Context, node *graphCopyNode) error {
	stack := []*graphCopyNode{node}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node.done = true
		if c.opts.Complete != nil {
			if err := c.opts.Complete(ctx, node.ref); err != nil {
				return err
			}
		}
		for _, parent := range node.parents {
			parent.waiting--
			if parent.waiting == 0 {
				stack = append(stack, parent)
			}
		}
		node.parents = nil
	}
	return nil
}

// known reports which refs already have a complete copy.
func (c *graphCopy) known(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	if c.opts.Known == nil {
		return make([]bool, len(refs)), nil
	}
	known, err := c.opts.Known(ctx, refs)
	if err != nil {
		return nil, err
	}
	if len(known) != len(refs) {
		return nil, errors.Errorf("known returned %d results for %d refs", len(known), len(refs))
	}
	return known, nil
}
