package s4db

import (
	"bytes"
	"slices"
	"sync"
)

// change is one key's change applied at a checkpoint.
type change struct {
	// key is the key.
	key []byte
	// del deletes the key.
	del bool
	// val is the value a set stores.
	val value
}

// child is a page in a parent being rebuilt.
type child struct {
	// low is the lowest key the child covers.
	low []byte
	// page is an existing page, when d is nil.
	page uint64
	// d is a new page.
	d *draft
}

// builder rewrites the pages a checkpoint changes. A new leaf is written as
// soon as the leaves under its parent are settled, so a checkpoint holds the
// entries of the leaves under one inner page at a time. New inner pages are
// drafts holding their new inner children until place writes them.
type builder struct {
	// p reads existing pages.
	p *pager
	// out writes the new pages.
	out *pageWriter
	// freed lists the replaced pages.
	freed []uint64
}

// build applies sorted changes to the tree at root and returns the new root,
// or nil when the tree is left empty.
func (b *builder) build(root uint64, changes []change) (*child, error) {
	// Rewrite the changed leaves and the pages above them.
	var kids []child
	if root == 0 {
		kids = packLeaves(mergeLeaf(nil, nil, changes))
	} else {
		var err error
		if kids, err = b.apply(root, nil, changes); err != nil {
			return nil, err
		}
	}
	if err := b.emit(kids); err != nil {
		return nil, err
	}

	// Stack inner pages until one root remains.
	for len(kids) > 1 {
		kids = packInner(kids)
	}
	if len(kids) == 0 {
		return nil, nil
	}

	// Drop new inner roots with a single child.
	top := kids[0]
	for top.d != nil && !top.d.leaf && len(top.d.keys) == 1 {
		if s := top.d.sub[0]; s != nil {
			top = child{d: s}
			continue
		}
		top = child{page: top.d.kids[0]}
	}
	return &top, nil
}

// apply rewrites page with changes, returning its replacement pages. low is
// the lowest key the page covers in its parent.
func (b *builder) apply(page uint64, low []byte, changes []change) ([]child, error) {
	// Replace the page.
	n, err := b.p.node(page)
	if err != nil {
		return nil, err
	}
	b.freed = append(b.freed, page)

	// A leaf merges its changes directly.
	if n.leaf {
		kids := packLeaves(mergeLeaf(nil, n, changes))
		if len(kids) != 0 && low != nil {
			kids[0].low = low
		}
		return kids, nil
	}

	// Split the changes among the children, and read the changed children
	// together: each read is random, and one at a time they bound the
	// checkpoint.
	ends, changed := splitChanges(n, changes)
	nodes, err := b.p.nodes(changed)
	if err != nil {
		return nil, err
	}

	// Read the changed children of changed inner children ahead of their
	// rewrite, so one child's reads overlap the rewrite of the one before.
	ra := b.readAhead(nodes, ends, changes)
	defer ra.close()
	inner := 0

	// Rewrite each child with changes in its range and keep the rest.
	// Adjacent rewritten leaves pack together, so the checkpoint, which
	// rewrites them anyway, leaves them as full as their entries allow.
	var kids []child
	var leaves []change
	var leavesLow []byte
	inRun := false
	flush := func() {
		if packed := packLeaves(leaves); len(packed) != 0 {
			packed[0].low = leavesLow
			kids = append(kids, packed...)
		}
		// packLeaves copies the entries into its drafts, so the run's
		// slice is reused for the next run.
		leaves, inRun = leaves[:0], false
	}
	start := 0
	for i, end := range ends {
		// Keep a child without changes. The first child's low is the
		// page's own.
		k, page := n.key(i), n.kid(i)
		if i == 0 {
			k = low
		}
		if end == start {
			flush()
			kids = append(kids, child{low: k, page: page})
			continue
		}

		// Add a changed leaf's entries to the run, or rewrite an inner
		// child.
		c := nodes[0]
		nodes = nodes[1:]
		if c.leaf {
			if !inRun {
				leavesLow, inRun = k, true
			}
			b.freed = append(b.freed, page)
			leaves = mergeLeaf(leaves, c, changes[start:end])
		} else {
			flush()
			ra.wait(inner)
			sub, err := b.apply(page, k, changes[start:end])
			if err != nil {
				return nil, err
			}
			ra.next()
			inner++
			kids = append(kids, sub...)
		}
		start = end
	}
	flush()

	// Merge underfull new children, write the new leaves among them, and
	// pack them into new inner pages.
	kids, err = b.rebalance(kids)
	if err != nil {
		return nil, err
	}
	if err := b.emit(kids); err != nil {
		return nil, err
	}
	out := packInner(kids)
	if len(out) != 0 && low != nil {
		out[0].low = low
	}
	return out, nil
}

// splitChanges returns where changes end for each child of inner page n,
// and the children with changes.
func splitChanges(n *node, changes []change) ([]int, []uint64) {
	// Find the end of each child's range in the sorted changes.
	ends := make([]int, n.count())
	var changed []uint64
	start := 0
	for i := range ends {
		ends[i] = len(changes)
		if i+1 < n.count() {
			ends[i], _ = slices.BinarySearchFunc(changes, n.key(i+1), func(c change, k []byte) int { return bytes.Compare(c.key, k) })
		}
		if ends[i] > start {
			changed = append(changed, n.kid(i))
		}
		start = ends[i]
	}
	return ends, changed
}

// readAhead returns a reader of the changed children of nodes, the changed
// children of one inner page, split by ends. It has nothing to read when
// nodes are leaves.
func (b *builder) readAhead(nodes []*node, ends []int, changes []change) *readAhead {
	// List the changed children of each changed inner child.
	ra := &readAhead{slots: make(chan struct{}, aheadBatches), stop: make(chan struct{})}
	if len(nodes) == 0 || nodes[0].leaf {
		return ra
	}
	var batches [][]uint64
	start := 0
	for _, end := range ends {
		if end > start {
			_, changed := splitChanges(nodes[len(batches)], changes[start:end])
			batches = append(batches, changed)
		}
		start = end
	}

	// Read them in order, up to aheadBatches past the rewrite, keeping
	// nodeReads reads in flight across the batches so a batch's last reads
	// overlap the next one's first. A failed read is left to the rewrite,
	// which reads the page again and reports the error.
	ra.ready = make([]chan struct{}, len(batches))
	for i := range ra.ready {
		ra.ready[i] = make(chan struct{})
	}
	ra.wg.Go(func() {
		reads := make(chan struct{}, nodeReads)
		for i, pages := range batches {
			// Wait for the rewrite to come within aheadBatches.
			select {
			case ra.slots <- struct{}{}:
			case <-ra.stop:
				return
			}

			// Start the batch's reads, and mark it ready when they end.
			var batch sync.WaitGroup
			for _, page := range pages {
				reads <- struct{}{}
				batch.Add(1)
				ra.wg.Go(func() {
					_, _ = b.p.node(page)
					<-reads
					batch.Done()
				})
			}
			ra.wg.Go(func() {
				batch.Wait()
				close(ra.ready[i])
			})
		}
	})
	return ra
}

// aheadBatches bounds the inner children whose changed children a
// readAhead holds read before their rewrite.
const aheadBatches = 4

// readAhead reads the changed children of an inner page's changed inner
// children into the cache in its own goroutine, in the order the build
// rewrites them.
type readAhead struct {
	// ready closes as each inner child's changed children are read.
	ready []chan struct{}
	// slots holds a token for each inner child read and not yet rewritten.
	slots chan struct{}
	// stop ends the reader.
	stop chan struct{}
	// wg waits for the reader.
	wg sync.WaitGroup
}

// wait waits until the changed children of the i-th changed inner child are
// read.
func (ra *readAhead) wait(i int) {
	if ra.ready != nil {
		<-ra.ready[i]
	}
}

// next lets the reader read one more inner child ahead, after the build
// rewrites one.
func (ra *readAhead) next() {
	if ra.ready != nil {
		<-ra.slots
	}
}

// close stops the reader and waits for it, so no read outlives the build's
// snapshot of the tree.
func (ra *readAhead) close() {
	close(ra.stop)
	ra.wg.Wait()
}

// rebalance merges each new child filled under half a page into a
// neighbor when the two fit in one page, so deletes do not leave sparse
// pages behind. Each merge removes a page, so the loop ends. Pages the
// checkpoint did not rewrite stay as they are.
func (b *builder) rebalance(kids []child) ([]child, error) {
	for i := 0; i < len(kids); i++ {
		// Skip existing and well filled children.
		if kids[i].d == nil || kids[i].d.fill() >= pageRoom/2 {
			continue
		}

		// Find a neighbor, right first, that fits in one page with it.
		lo := -1
		for _, j := range []int{i, i - 1} {
			if j < 0 || j+1 >= len(kids) {
				continue
			}
			fits, err := b.fits(kids[j], kids[j+1])
			if err != nil {
				return nil, err
			}
			if fits {
				lo = j
				break
			}
		}
		if lo < 0 {
			continue
		}

		// Merge the pair and look at the result again.
		merged, err := b.merge(kids[lo], kids[lo+1])
		if err != nil {
			return nil, err
		}
		kids = slices.Replace(kids, lo, lo+2, merged...)
		i = lo - 1
	}
	return kids, nil
}

// fill returns the bytes the entries of c occupy and whether c is a leaf.
func (b *builder) fill(c child) (int, bool, error) {
	if c.d != nil {
		return c.d.fill(), c.d.leaf, nil
	}
	n, err := b.p.node(c.page)
	if err != nil {
		return 0, false, err
	}
	return n.fill(), n.leaf, nil
}

// fits reports whether the entries of two children fit in one page. The
// first key of an inner page is empty, so merging inner pages adds the right
// one's low key.
func (b *builder) fits(l, r child) (bool, error) {
	// Sum the entries of both pages.
	total := 0
	leaf := false
	for _, c := range []child{l, r} {
		n, isLeaf, err := b.fill(c)
		if err != nil {
			return false, err
		}
		total += n
		leaf = isLeaf
	}
	if !leaf {
		total += innerEntrySize(r.low) - innerEntrySize(nil)
	}
	return total <= pageRoom, nil
}

// merge repacks the entries of two adjacent children of one level into new
// pages, replacing any existing page among them.
func (b *builder) merge(l, r child) ([]child, error) {
	// Draft existing pages, which the new ones replace.
	pair := []child{l, r}
	var drafts [2]*draft
	for i, c := range pair {
		drafts[i] = c.d
		if c.d != nil {
			continue
		}
		n, err := b.p.node(c.page)
		if err != nil {
			return nil, err
		}
		b.freed = append(b.freed, c.page)
		drafts[i] = draftOf(n)
	}

	// Leaves repack their entries.
	var out []child
	if drafts[0].leaf {
		var entries []change
		for _, d := range drafts {
			for j, k := range d.keys {
				entries = append(entries, change{key: k, val: d.vals[j]})
			}
		}
		out = packLeaves(entries)
		out[0].low = l.low
		return out, nil
	}

	// Inner pages repack their children.
	var kids []child
	for i, d := range drafts {
		for j, k := range d.keys {
			c := child{low: k, page: d.kids[j], d: d.sub[j]}
			if j == 0 {
				c.low = pair[i].low
			}
			kids = append(kids, c)
		}
	}
	out = packInner(kids)
	out[0].low = l.low
	return out, nil
}

// mergeLeaf appends to out the sorted changes merged into the entries of
// leaf n, which may be nil for an empty tree.
func mergeLeaf(out []change, n *node, changes []change) []change {
	// Interleave existing entries with the changes, which win on equal keys.
	count := 0
	if n != nil {
		count = n.count()
	}
	out = slices.Grow(out, count+len(changes))
	i := 0
	for _, c := range changes {
		for ; i < count; i++ {
			k := n.key(i)
			if bytes.Compare(k, c.key) >= 0 {
				break
			}
			out = append(out, change{key: k, val: n.val(i)})
		}
		if i < count && bytes.Equal(n.key(i), c.key) {
			i++
		}
		if !c.del {
			out = append(out, c)
		}
	}

	// Keep the entries after the last change.
	for ; i < count; i++ {
		out = append(out, change{key: n.key(i), val: n.val(i)})
	}
	return out
}

// split returns the end of each page when items of the given sizes are
// spread evenly over the fewest pages that hold them.
func split(sizes []int) []int {
	// Aim each page at an even share of the total.
	total := 0
	for _, s := range sizes {
		total += s
	}
	target := total / max(1, (total+pageRoom-1)/pageRoom)

	// End a page when the next item would overflow it or it reached its
	// share.
	var ends []int
	cur := 0
	for i, s := range sizes {
		if cur > 0 && (cur+s > pageRoom || cur >= target) {
			ends = append(ends, i)
			cur = 0
		}
		cur += s
	}
	if len(sizes) != 0 {
		ends = append(ends, len(sizes))
	}
	return ends
}

// packLeaves packs entries into new leaf pages.
func packLeaves(entries []change) []child {
	// Measure the entries.
	sizes := make([]int, len(entries))
	for i, e := range entries {
		sizes[i] = leafEntrySize(e.key, e.val)
	}

	// Fill a leaf for each split.
	var kids []child
	start := 0
	for _, end := range split(sizes) {
		d := &draft{leaf: true, keys: make([][]byte, 0, end-start), vals: make([]value, 0, end-start)}
		for _, e := range entries[start:end] {
			d.keys = append(d.keys, e.key)
			d.vals = append(d.vals, e.val)
		}
		kids = append(kids, child{low: d.keys[0], d: d})
		start = end
	}
	return kids
}

// packInner packs children into new inner pages. Each page stores an empty
// first key and copies the others into one buffer, so a draft held until
// place keeps no replaced page alive.
func packInner(kids []child) []child {
	// Measure the entries. The first is stored empty; later pages measure
	// their first key, which leaves them a little room.
	sizes := make([]int, len(kids))
	for i, k := range kids {
		sizes[i] = innerEntrySize(k.low)
	}
	if len(sizes) != 0 {
		sizes[0] = innerEntrySize(nil)
	}

	// Fill an inner page for each split.
	var out []child
	start := 0
	for _, end := range split(sizes) {
		keyBytes := 0
		for _, k := range kids[start+1 : end] {
			keyBytes += len(k.low)
		}
		buf := make([]byte, 0, keyBytes)
		d := &draft{keys: make([][]byte, 0, end-start), kids: make([]uint64, 0, end-start), sub: make([]*draft, 0, end-start)}
		for i, k := range kids[start:end] {
			key := buf[len(buf):]
			if i != 0 {
				buf = append(buf, k.low...)
				key = buf[len(buf)-len(k.low):]
			}
			d.keys = append(d.keys, key)
			d.kids = append(d.kids, k.page)
			d.sub = append(d.sub, k.d)
		}
		out = append(out, child{low: kids[start].low, d: d})
		start = end
	}
	return out
}

// emit writes the new leaves among kids and replaces each with its page.
func (b *builder) emit(kids []child) error {
	for i, c := range kids {
		if c.d == nil || !c.d.leaf {
			continue
		}
		page, err := b.out.add(c.d)
		if err != nil {
			return err
		}
		kids[i] = child{low: c.low, page: page}
	}
	return nil
}

// place writes the new inner pages under top, children first, sets top's
// page, and writes the pages still buffered.
func (b *builder) place(top *child) error {
	// Write each draft after its new children, resolving their pages.
	var visit func(d *draft) (uint64, error)
	visit = func(d *draft) (uint64, error) {
		for j, s := range d.sub {
			if s == nil {
				continue
			}
			page, err := visit(s)
			if err != nil {
				return 0, err
			}
			d.kids[j] = page
		}
		return b.out.add(d)
	}
	if top != nil && top.d != nil {
		page, err := visit(top.d)
		if err != nil {
			return err
		}
		*top = child{page: page}
	}
	return b.out.flush()
}
