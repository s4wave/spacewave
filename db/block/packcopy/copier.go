// Package packcopy copies what a set of roots reaches into a pack store.
//
// A copy reads each block once, parents first, follows the refs stored with
// it, and fills whole packs. It never decodes a block. A block no source can
// serve becomes a hole: the walk goes on, the hole goes on a list, and the
// blocks above it stay unproven until a later pass fills it.
//
// A proof says a block and everything under it are durable in the target.
// Proofs commit only after the packs that hold their blocks, so a copy killed
// at any point resumes from its proofs with none running ahead of the data.
package packcopy

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// DefaultReads is the number of block reads a copy keeps in flight when the
// options set none. A source's own connection limit decides how many run at
// once; the rest wait there.
const DefaultReads = 32

// Reasons a block is a hole, as the holes list names them.
const (
	// ReasonMissing is a block no source holds.
	ReasonMissing = "missing"
	// ReasonCorrupt is a block whose bytes fail their hash in every source
	// that held it.
	ReasonCorrupt = "corrupt"
	// ReasonRefsUnknown is a block held only without its refs. A pack cannot
	// store that, and a leaf would hide its children.
	ReasonRefsUnknown = "refs-unknown"
	// ReasonError is a block whose reads failed.
	ReasonError = "error"
)

// Source is a store a copy reads blocks from.
type Source interface {
	// GetStoredBlock returns the block with its refs, or nil when the
	// source does not hold it.
	GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error)
}

// Target is the pack store a copy writes to. PackStore implements it.
type Target interface {
	Source
	// LocateBlocks reports, for each ref, whether the target's index places
	// it in a pack.
	LocateBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
	// GetBatchLimits returns the size of one pack.
	GetBatchLimits() block.BatchLimits
	// PutBlockBatch writes the batch as one durable pack.
	PutBlockBatch(ctx context.Context, batch []*block.PutBatchEntry) ([]bool, error)
}

// Options configures Copy.
type Options struct {
	// Roots are the blocks to copy, with everything under them.
	Roots []*block.BlockRef
	// Target receives the blocks.
	Target Target
	// Sources serve the blocks the target lacks, tried in order.
	Sources []Source
	// Proofs holds the proofs, across runs. The copy owns every key in it,
	// so pass a prefixed view.
	Proofs kvtx.Store
	// Graph receives a record of each block before its pack is written, nil
	// for none. A pack written with no proof yet is read back on resume and
	// not written again, so its records must already be stored.
	Graph *GraphStream
	// Holes receives each hole once with its reason.
	Holes func(ref *block.BlockRef, reason string) error
	// Reads is the number of reads kept in flight. Zero uses DefaultReads.
	Reads int
	// Uploads is the number of packs written at once, which is what fills a
	// fast link: one pack is one PUT. Each holds about three copies of its
	// blocks in memory while it writes. Zero uses the target's batch limits.
	Uploads int
	// Progress observes the counts after each pack, nil for none.
	Progress func(Result)
}

// Result reports a copy.
type Result struct {
	// Read is the blocks read from the sources.
	Read uint64
	// ReadBytes is the size of those blocks.
	ReadBytes uint64
	// Held is the blocks the target already held, read back for their refs.
	Held uint64
	// Written is the blocks written to durable packs.
	Written uint64
	// WrittenBytes is the size of those blocks.
	WrittenBytes uint64
	// Packs is the packs written.
	Packs uint64
	// Proven is the blocks proven by this run.
	Proven uint64
	// Holes counts the holes by reason.
	Holes map[string]uint64
	// Complete is set when every root is proven.
	Complete bool
}

// Copy copies what opts.Roots reach into opts.Target and returns what it did.
// A hole is not an error: Result.Complete stays false, and another pass with
// a source that holds the block finishes the copy.
//
// An error stops the walk where it stands. What was read is still written and
// proven, so the next run resumes from there. It returns only after every
// reader has returned and the last pack is durable.
func Copy(ctx context.Context, opts *Options) (*Result, error) {
	c := &copier{
		opts:    opts,
		reads:   opts.Reads,
		uploads: opts.Uploads,
		limits:  opts.Target.GetBatchLimits(),
		nodes:   make(map[string]*node),
		proving: make(map[string]struct{}),
		flushes: make(chan *flush, 1),
		result:  Result{Holes: make(map[string]uint64)},
	}
	if c.reads <= 0 {
		c.reads = DefaultReads
	}
	if c.uploads <= 0 {
		c.uploads = max(c.limits.Workers, 1)
	}
	return c.run(ctx)
}

// copier holds the state of one Copy call. Only the calling goroutine touches
// nodes, queue, batch and proofs; readers report through results and the
// flusher works through flushes.
type copier struct {
	// opts configures the copy.
	opts *Options
	// reads is the number of reads kept in flight.
	reads int
	// uploads is the number of packs written at once.
	uploads int
	// limits is the size of one pack.
	limits block.BatchLimits

	// nodes holds the blocks being walked by marshaled ref, and the blocks a
	// hole sits under, which later parents must inherit. A proven block
	// leaves it.
	nodes map[string]*node
	// queue holds blocks waiting for a read, popped last in first out so the
	// walk runs depth first and proves subtrees early.
	queue []*node
	// batch collects the next pack in read order, parents first.
	batch []*block.PutBatchEntry
	// batchBytes is the size of batch.
	batchBytes int
	// proofs lists the blocks proven since the last flush.
	proofs []*block.BlockRef

	// provingMtx guards proving.
	provingMtx sync.Mutex
	// proving holds the blocks proven but not yet committed, so a later
	// parent need not walk them again.
	proving map[string]struct{}

	// flushes carries packs to the flusher, in order.
	flushes chan *flush
	// resultMtx guards result.
	resultMtx sync.Mutex
	// result is the counts so far.
	result Result
}

// node is one block of the walk.
type node struct {
	// ref is the block.
	ref *block.BlockRef
	// held is set when the target's index places the block.
	held bool
	// waiting counts children not yet done.
	waiting int
	// parents wait for this block.
	parents []*node
	// done is set once the block and its subtree are walked.
	done bool
	// holed is set when a hole sits at or under the block.
	holed bool
}

// flush is one pack and the proofs it makes durable.
type flush struct {
	// batch is the pack's blocks, empty when only proofs remain.
	batch []*block.PutBatchEntry
	// bytes is the size of batch.
	bytes int
	// proofs commit once batch and every earlier batch are durable.
	proofs []*block.BlockRef
	// uploaded receives the end of the pack's upload.
	uploaded chan error
}

// readResult is the end of one block read.
type readResult struct {
	// node is the block.
	node *node
	// stored is the block, nil for a hole.
	stored *block.StoredBlock
	// held is set when stored came from the target.
	held bool
	// reason says why the block is a hole.
	reason string
	// err stops the walk: the block is neither read nor a hole.
	err error
}

// run walks the roots, then waits for the last pack.
func (c *copier) run(ctx context.Context) (*Result, error) {
	// Commit packs behind the walk. A stop still commits what was read, so
	// the flusher does not end with ctx, and a failed pack stops the walk.
	walkCtx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	flushed := make(chan error, 1)
	go func() {
		err := c.runFlusher(context.WithoutCancel(ctx), stop)
		flushed <- err
	}()

	// Walk, then hand over the rest. A pack smaller than full goes only here.
	err := c.walk(walkCtx)
	c.flushes <- c.takeFlush()
	close(c.flushes)
	if ferr := <-flushed; ferr != nil {
		err = ferr
	}
	if err != nil {
		result := c.snapshot()
		return &result, err
	}

	// Report the roots' state from the proofs.
	proven, err := c.proven(ctx, c.opts.Roots)
	if err != nil {
		return nil, err
	}
	result := c.snapshot()
	result.Complete = !slices.Contains(proven, false)
	return &result, nil
}

// walk runs the reader pool until the queue drains or the walk stops. It
// returns only after every reader it started has returned.
func (c *copier) walk(ctx context.Context) error {
	// Seed the queue with the roots.
	roots := make([]*node, 0, len(c.opts.Roots))
	for _, ref := range c.opts.Roots {
		if ref.GetEmpty() {
			continue
		}
		key := ref.MarshalString()
		if c.nodes[key] != nil {
			continue
		}
		n := &node{ref: ref}
		c.nodes[key] = n
		roots = append(roots, n)
	}
	if _, err := c.enqueue(ctx, roots); err != nil {
		return err
	}

	// Read blocks depth first, handling one result at a time.
	results := make(chan readResult, c.reads)
	inFlight := 0
	var stop error
	for {
		// Fill the pool, unless the walk is stopping.
		if stop == nil {
			stop = context.Cause(ctx)
		}
		for stop == nil && inFlight < c.reads && len(c.queue) != 0 {
			n := c.queue[len(c.queue)-1]
			c.queue = c.queue[:len(c.queue)-1]
			inFlight++
			go func() { results <- c.read(ctx, n) }()
		}
		if inFlight == 0 {
			return stop
		}

		// Handle one result; a read in flight when the walk stopped still
		// counts when it succeeded.
		res := <-results
		inFlight--
		if err := c.handle(ctx, res); err != nil && stop == nil {
			stop = err
		}
	}
}

// read finds a block in the target, then in each source. A block whose bytes
// fail their hash counts as absent from that store, so a later source can
// still serve it and a corrupt copy in the target is replaced.
func (c *copier) read(ctx context.Context, n *node) readResult {
	// Read a block the target holds back from the target.
	res := readResult{node: n, reason: ReasonMissing}
	if n.held {
		stored, reason, err := readFrom(ctx, c.opts.Target, n.ref)
		if err != nil {
			return readResult{node: n, err: err}
		}
		if reason == "" {
			return readResult{node: n, stored: stored, held: true}
		}
		res.reason = worseReason(res.reason, reason)
	}

	// Try each source in order.
	for _, src := range c.opts.Sources {
		stored, reason, err := readFrom(ctx, src, n.ref)
		if err != nil {
			return readResult{node: n, err: err}
		}
		if reason == "" {
			return readResult{node: n, stored: stored}
		}
		res.reason = worseReason(res.reason, reason)
	}
	return res
}

// readFrom reads and checks a block from one store, returning the reason the
// store cannot serve it. An error stops the walk: the run is over, or the read
// went over the budget. Any other failure is that store's.
func readFrom(ctx context.Context, src Source, ref *block.BlockRef) (*block.StoredBlock, string, error) {
	// Read the block, separating a stop from a failed read.
	stored, err := src.GetStoredBlock(ctx, ref)
	if ctx.Err() != nil {
		return nil, "", context.Cause(ctx)
	}
	if errors.Is(err, ErrBudget) {
		return nil, "", err
	}
	if block.IsNotAvailable(err) {
		return nil, ReasonMissing, nil
	}
	if err != nil {
		return nil, ReasonError, nil
	}
	if stored == nil {
		return nil, ReasonMissing, nil
	}

	// Check the bytes, then the refs the walk follows.
	if err := ref.VerifyData(stored.Data, false); err != nil {
		return nil, ReasonCorrupt, nil
	}
	if !stored.RefsKnown {
		return nil, ReasonRefsUnknown, nil
	}
	return stored, "", nil
}

// worseReason returns the reason that says more about a block no store could
// serve: a store that held it outranks one that did not.
func worseReason(a, b string) string {
	if a == ReasonMissing {
		return b
	}
	return a
}

// handle takes one read result into the walk.
func (c *copier) handle(ctx context.Context, res readResult) error {
	// A stopped read leaves the block and the blocks above it unproven.
	if res.err != nil {
		return res.err
	}

	// A hole marks the blocks above it.
	if res.stored == nil {
		res.node.holed = true
		c.count(func(r *Result) { r.Holes[res.reason]++ })
		if err := c.opts.Holes(res.node.ref, res.reason); err != nil {
			return err
		}
		c.finish(res.node)
		return nil
	}

	// Add the block to the next pack, parents first.
	if res.held {
		c.count(func(r *Result) { r.Held++ })
	} else {
		c.count(func(r *Result) {
			r.Read++
			r.ReadBytes += uint64(len(res.stored.Data))
		})
		c.addBlock(res.node.ref, res.stored)
	}

	// Follow the refs.
	return c.expand(ctx, res.node, res.stored.Refs)
}

// addBlock appends a block to the batch, first sending the batch when the
// block would overfill it.
func (c *copier) addBlock(ref *block.BlockRef, stored *block.StoredBlock) {
	// Send a full batch.
	size := len(stored.Data)
	if len(c.batch) != 0 && (int64(c.batchBytes+size) > c.limits.Bytes || (c.limits.Blocks != 0 && len(c.batch) >= c.limits.Blocks)) {
		c.flushes <- c.takeFlush()
	}

	// Hold the block for the next pack.
	c.batch = append(c.batch, &block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: true})
	c.batchBytes += size
}

// takeFlush returns the batch and the proofs so far. Every block under a proof
// was added to this batch or an earlier one, or was proven before it, so the
// proofs are true once the batch is durable.
func (c *copier) takeFlush() *flush {
	f := &flush{batch: c.batch, bytes: c.batchBytes, proofs: c.proofs}
	c.batch, c.batchBytes, c.proofs = nil, 0, nil
	return f
}

// expand links a read block to its children, queues those still to walk, and
// finishes the block when none remain.
func (c *copier) expand(ctx context.Context, n *node, refs []*block.BlockRef) error {
	// Link the children, collecting the ones not seen in this run.
	var fresh []*node
	for _, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		key := ref.MarshalString()
		child := c.nodes[key]
		if child == nil {
			child = &node{ref: ref}
			c.nodes[key] = child
			fresh = append(fresh, child)
		}

		// Only a block with a hole under it stays in nodes once done.
		if child.done {
			n.holed = true
			continue
		}

		// A block listing a child twice waits for it once.
		if last := len(child.parents) - 1; last < 0 || child.parents[last] != n {
			child.parents = append(child.parents, n)
			n.waiting++
		}
	}

	// Queue the fresh children and drop the proven ones.
	proven, err := c.enqueue(ctx, fresh)
	if err != nil {
		return err
	}
	for _, child := range proven {
		child.parents = nil
		n.waiting--
	}

	// Finish a block with nothing left under it.
	if n.waiting == 0 {
		c.finish(n)
	}
	return nil
}

// enqueue queues the nodes that are not proven, noting which of them the
// target holds, and returns those that are proven, forgotten from the walk.
func (c *copier) enqueue(ctx context.Context, nodes []*node) ([]*node, error) {
	// Ask the proofs and the target's index about all of them at once.
	if len(nodes) == 0 {
		return nil, nil
	}
	refs := make([]*block.BlockRef, len(nodes))
	for i, n := range nodes {
		refs[i] = n.ref
	}
	proven, err := c.proven(ctx, refs)
	if err != nil {
		return nil, err
	}
	held, err := c.opts.Target.LocateBlocks(ctx, refs)
	if err != nil {
		return nil, errors.Wrap(err, "locate blocks")
	}

	// Split the nodes.
	var skipped []*node
	for i, n := range nodes {
		if proven[i] {
			delete(c.nodes, n.ref.MarshalString())
			skipped = append(skipped, n)
			continue
		}
		n.held = held[i]
		c.queue = append(c.queue, n)
	}
	return skipped, nil
}

// proven reports which refs have a proof, committed or on its way.
func (c *copier) proven(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Check the proofs on their way to the store.
	out := make([]bool, len(refs))
	c.provingMtx.Lock()
	for i, ref := range refs {
		_, out[i] = c.proving[ref.MarshalString()]
	}
	c.provingMtx.Unlock()

	// Check the rest in one read.
	tx, err := c.opts.Proofs.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	for i, ref := range refs {
		if out[i] {
			continue
		}
		_, out[i], err = tx.Get(ctx, packfile.BlockKey(ref.GetHash()))
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// finish completes a walked block, then each parent it released. A block with
// no hole under it is proven and leaves the walk; one with a hole stays, so a
// parent that reaches it later inherits the hole.
func (c *copier) finish(n *node) {
	stack := []*node{n}
	for len(stack) != 0 {
		// Complete the block.
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n.done = true
		if !n.holed {
			c.prove(n)
		}

		// Release each parent.
		for _, parent := range n.parents {
			parent.holed = parent.holed || n.holed
			parent.waiting--
			if parent.waiting == 0 {
				stack = append(stack, parent)
			}
		}
		n.parents = nil
	}
}

// prove records a block as proven in this run and forgets it.
func (c *copier) prove(n *node) {
	// Move the block from the walk to the next pack's proofs.
	key := n.ref.MarshalString()
	delete(c.nodes, key)
	c.proofs = append(c.proofs, n.ref)

	// Mark it pending until the proof commits.
	c.provingMtx.Lock()
	c.proving[key] = struct{}{}
	c.provingMtx.Unlock()
}

// runFlusher uploads up to c.uploads packs at once and commits their proofs in
// order, each only after its own pack and every earlier one are durable. A
// proof names blocks in earlier packs too, so a later pack finishing first
// must not commit its proofs. A failure stops the walk and drains the rest:
// their proofs stay uncommitted, which costs only a repeated walk.
func (c *copier) runFlusher(ctx context.Context, stop context.CancelCauseFunc) error {
	// Commit in order. The queue holds the uploads behind the one the
	// committer waits on, which bounds how many run.
	queue := make(chan *flush, c.uploads-1)
	committed := make(chan error, 1)
	go func() {
		var failed error
		for f := range queue {
			err := <-f.uploaded
			if failed != nil {
				continue
			}
			if err == nil {
				err = c.commitProofs(ctx, f)
			}
			if failed = err; failed != nil {
				stop(failed)
			}
		}
		committed <- failed
	}()

	// Start each upload once it has a place in the queue.
	for f := range c.flushes {
		f.uploaded = make(chan error, 1)
		queue <- f
		go func() { f.uploaded <- c.upload(ctx, f) }()
	}
	close(queue)
	return <-committed
}

// upload stores a flush's graph records, then its pack. The records go first:
// a pack with no proof yet is read back on resume and not written again.
func (c *copier) upload(ctx context.Context, f *flush) error {
	if len(f.batch) == 0 {
		return nil
	}
	if c.opts.Graph != nil {
		if err := c.opts.Graph.Write(ctx, f.batch); err != nil {
			return errors.Wrap(err, "write graph records")
		}
	}
	if _, err := c.opts.Target.PutBlockBatch(ctx, f.batch); err != nil {
		return errors.Wrap(err, "write pack")
	}
	return nil
}

// commitProofs commits the proofs of a durable flush and reports it.
func (c *copier) commitProofs(ctx context.Context, f *flush) error {
	// Commit the proofs in one transaction.
	if len(f.proofs) != 0 {
		tx, err := c.opts.Proofs.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		for _, ref := range f.proofs {
			if err := tx.Set(ctx, packfile.BlockKey(ref.GetHash()), []byte{1}); err != nil {
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return errors.Wrap(err, "commit proofs")
		}
	}

	// Forget the proofs the store now holds.
	c.provingMtx.Lock()
	for _, ref := range f.proofs {
		delete(c.proving, ref.MarshalString())
	}
	c.provingMtx.Unlock()

	// Count the flush and report.
	c.count(func(r *Result) {
		r.Written += uint64(len(f.batch))
		r.WrittenBytes += uint64(f.bytes) //nolint:gosec // a byte count is never negative.
		r.Proven += uint64(len(f.proofs))
		if len(f.batch) != 0 {
			r.Packs++
		}
	})
	if c.opts.Progress != nil {
		c.opts.Progress(c.snapshot())
	}
	return nil
}

// count updates the result.
func (c *copier) count(update func(*Result)) {
	c.resultMtx.Lock()
	defer c.resultMtx.Unlock()
	update(&c.result)
}

// snapshot returns a copy of the counts.
func (c *copier) snapshot() Result {
	// Hold the counts, then copy them with their map.
	c.resultMtx.Lock()
	defer c.resultMtx.Unlock()
	result := c.result
	result.Holes = maps.Clone(c.result.Holes)
	return result
}
