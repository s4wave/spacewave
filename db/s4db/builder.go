package s4db

import (
	"bytes"
	"slices"
)

// placeBatch is the number of pages place encodes before writing them.
const placeBatch = 256

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
	// page is an existing page, when n is nil.
	page uint64
	// n is a new page.
	n *node
}

// builder rewrites the pages a checkpoint changes. New pages hold their
// children as nodes until pages are assigned.
type builder struct {
	// p reads existing pages.
	p *pager
	// freed lists the replaced pages.
	freed []uint64
	// sub holds the new children of each new inner page, nil where the
	// child is an existing page.
	sub map[*node][]*node
}

// newBuilder returns a builder reading existing pages from p.
func newBuilder(p *pager) *builder {
	return &builder{p: p, sub: make(map[*node][]*node)}
}

// build applies sorted changes to the tree at root and returns the new root,
// or nil when the tree is left empty.
func (b *builder) build(root uint64, changes []change) (*child, error) {
	// Rewrite the changed leaves and the pages above them.
	var kids []child
	if root == 0 {
		kids = b.packLeaves(mergeLeaf(nil, nil, changes))
	} else {
		var err error
		if kids, err = b.apply(root, nil, changes); err != nil {
			return nil, err
		}
	}

	// Stack inner pages until one root remains.
	for len(kids) > 1 {
		kids = b.packInner(kids)
	}
	if len(kids) == 0 {
		return nil, nil
	}

	// Drop new inner roots with a single child.
	top := kids[0]
	for top.n != nil && !top.n.leaf && len(top.n.kids) == 1 {
		if s := b.sub[top.n][0]; s != nil {
			top = child{n: s}
			continue
		}
		top = child{page: top.n.kids[0]}
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
		kids := b.packLeaves(mergeLeaf(n.keys, n.vals, changes))
		if len(kids) != 0 && low != nil {
			kids[0].low = low
		}
		return kids, nil
	}

	// Rewrite each child with changes in its range and keep the rest.
	// Adjacent rewritten leaves pack together, so the checkpoint, which
	// rewrites them anyway, leaves them as full as their entries allow.
	var kids []child
	var leaves []change
	var leavesLow []byte
	inRun := false
	flush := func() {
		if packed := b.packLeaves(leaves); len(packed) != 0 {
			packed[0].low = leavesLow
			kids = append(kids, packed...)
		}
		leaves, inRun = nil, false
	}
	for i, k := range n.keys {
		// Keep a child without changes.
		j := len(changes)
		if i+1 < len(n.keys) {
			j, _ = slices.BinarySearchFunc(changes, n.keys[i+1], func(c change, k []byte) int { return bytes.Compare(c.key, k) })
		}
		if j == 0 {
			flush()
			kids = append(kids, child{low: k, page: n.kids[i]})
			continue
		}

		// Add a changed leaf's entries to the run, or rewrite an inner
		// child.
		c, err := b.p.node(n.kids[i])
		if err != nil {
			return nil, err
		}
		if c.leaf {
			if !inRun {
				leavesLow, inRun = k, true
			}
			b.freed = append(b.freed, n.kids[i])
			leaves = append(leaves, mergeLeaf(c.keys, c.vals, changes[:j])...)
		} else {
			flush()
			sub, err := b.apply(n.kids[i], k, changes[:j])
			if err != nil {
				return nil, err
			}
			kids = append(kids, sub...)
		}
		changes = changes[j:]
	}
	flush()

	// Merge underfull new children and pack them into new inner pages.
	kids, err = b.rebalance(kids)
	if err != nil {
		return nil, err
	}
	out := b.packInner(kids)
	if len(out) != 0 && low != nil {
		out[0].low = low
	}
	return out, nil
}

// rebalance merges each new child filled under half a page into a
// neighbor when the two fit in one page, so deletes do not leave sparse
// pages behind. Each merge removes a page, so the loop ends. Pages the
// checkpoint did not rewrite stay as they are.
func (b *builder) rebalance(kids []child) ([]child, error) {
	for i := 0; i < len(kids); i++ {
		// Skip existing and well filled children.
		if kids[i].n == nil || kids[i].n.fill() >= pageRoom/2 {
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

// load returns the node of c, reading an existing page.
func (b *builder) load(c child) (*node, error) {
	if c.n != nil {
		return c.n, nil
	}
	return b.p.node(c.page)
}

// fits reports whether the entries of two children fit in one page.
func (b *builder) fits(l, r child) (bool, error) {
	total := 0
	for _, c := range []child{l, r} {
		n, err := b.load(c)
		if err != nil {
			return false, err
		}
		total += n.fill()
	}
	return total <= pageRoom, nil
}

// merge repacks the entries of two adjacent children of one level into new
// pages, replacing any existing page among them.
func (b *builder) merge(l, r child) ([]child, error) {
	// Load existing pages, which the new ones replace.
	pair := []child{l, r}
	var nodes [2]*node
	for i, c := range pair {
		n, err := b.load(c)
		if err != nil {
			return nil, err
		}
		if c.n == nil {
			b.freed = append(b.freed, c.page)
		}
		nodes[i] = n
	}

	// Leaves repack their entries.
	var out []child
	if nodes[0].leaf {
		var entries []change
		for _, n := range nodes {
			for j, k := range n.keys {
				entries = append(entries, change{key: k, val: n.vals[j]})
			}
		}
		out = b.packLeaves(entries)
		out[0].low = l.low
		return out, nil
	}

	// Inner pages repack their children.
	var kids []child
	for i, n := range nodes {
		subs := b.sub[n]
		for j, k := range n.keys {
			c := child{low: k, page: n.kids[j]}
			if j == 0 {
				c.low = pair[i].low
			}
			if subs != nil {
				c.n = subs[j]
			}
			kids = append(kids, c)
		}
	}
	out = b.packInner(kids)
	out[0].low = l.low
	return out, nil
}

// mergeLeaf merges sorted changes into sorted leaf entries.
func mergeLeaf(keys [][]byte, vals []value, changes []change) []change {
	// Interleave existing entries with the changes, which win on equal keys.
	out := make([]change, 0, len(keys)+len(changes))
	i := 0
	for _, c := range changes {
		for i < len(keys) && bytes.Compare(keys[i], c.key) < 0 {
			out = append(out, change{key: keys[i], val: vals[i]})
			i++
		}
		if i < len(keys) && bytes.Equal(keys[i], c.key) {
			i++
		}
		if !c.del {
			out = append(out, c)
		}
	}

	// Keep the entries after the last change.
	for ; i < len(keys); i++ {
		out = append(out, change{key: keys[i], val: vals[i]})
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
func (b *builder) packLeaves(entries []change) []child {
	// Measure the entries.
	sizes := make([]int, len(entries))
	for i, e := range entries {
		sizes[i] = leafEntrySize(e.key, e.val)
	}

	// Fill a leaf for each split.
	var kids []child
	start := 0
	for _, end := range split(sizes) {
		n := &node{leaf: true}
		for _, e := range entries[start:end] {
			n.keys = append(n.keys, e.key)
			n.vals = append(n.vals, e.val)
		}
		kids = append(kids, child{low: n.keys[0], n: n})
		start = end
	}
	return kids
}

// packInner packs children into new inner pages.
func (b *builder) packInner(kids []child) []child {
	// Measure the entries.
	sizes := make([]int, len(kids))
	for i, k := range kids {
		sizes[i] = innerEntrySize(k.low)
	}

	// Fill an inner page for each split.
	var out []child
	start := 0
	for _, end := range split(sizes) {
		n := &node{}
		subs := make([]*node, 0, end-start)
		for _, k := range kids[start:end] {
			n.keys = append(n.keys, k.low)
			n.kids = append(n.kids, k.page)
			subs = append(subs, k.n)
		}
		b.sub[n] = subs
		out = append(out, child{low: n.keys[0], n: n})
		start = end
	}
	return out
}

// place assigns consecutive pages from first to the new pages reachable from
// top, children first, passes their encodings to write in batches, and
// returns the nodes in page order; the last is the root.
func (b *builder) place(top *child, first uint64, write func(buf []byte, page uint64) error) ([]*node, error) {
	// Number the new pages in post order.
	if top.n == nil {
		return nil, nil
	}
	pages := make(map[*node]uint64)
	var order []*node
	var visit func(n *node)
	visit = func(n *node) {
		for _, s := range b.sub[n] {
			if s != nil {
				visit(s)
			}
		}
		pages[n] = first + uint64(len(order))
		order = append(order, n)
	}
	visit(top.n)

	// Resolve new children to their pages, encode, and write a batch of
	// pages at a time. Each returned node is decoded from its own page, so a
	// cached node never holds the buffers of the pages it replaced.
	buf := make([]byte, 0, placeBatch*pageSize)
	at := first
	placed := make(map[*node]*node, len(order))
	out := make([]*node, len(order))
	for i, n := range order {
		for j, s := range b.sub[n] {
			if s != nil {
				n.kids[j] = pages[s]
			}
		}
		buf = n.encode(buf)

		// Decode the page and link its new inner children.
		d, err := decodeNode(bytes.Clone(buf[len(buf)-pageSize:]))
		if err != nil {
			return nil, err
		}
		for j, s := range b.sub[n] {
			if s != nil && !s.leaf {
				d.inner[j].Store(placed[s])
			}
		}
		placed[n], out[i] = d, d

		// Write a full batch.
		if len(buf) == cap(buf) {
			if err := write(buf, at); err != nil {
				return nil, err
			}
			at += placeBatch
			buf = buf[:0]
		}
	}

	// Write the last partial batch.
	if len(buf) != 0 {
		if err := write(buf, at); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// reachable counts the new pages reachable from top.
func (b *builder) reachable(top *child) int {
	if top == nil || top.n == nil {
		return 0
	}
	var count func(n *node) int
	count = func(n *node) int {
		c := 1
		for _, s := range b.sub[n] {
			if s != nil {
				c += count(s)
			}
		}
		return c
	}
	return count(top.n)
}
