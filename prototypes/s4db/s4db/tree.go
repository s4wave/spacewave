package s4db

import (
	"bytes"
	"slices"
	"sync/atomic"
)

// pager reads tree pages.
type pager interface {
	// node returns the decoded page.
	node(page uint64) (*node, error)
	// root returns the decoded root page.
	root(page uint64) (*node, error)
	// child returns child i of inner page n.
	child(n *node, i int) (*node, error)
}

// childIndex returns the child of inner page n covering key.
func childIndex(n *node, key []byte) int {
	i, found := n.search(key)
	if found {
		return i
	}
	return max(0, i-1)
}

// treeGet looks key up in the tree at root.
func treeGet(p pager, root uint64, key []byte) (value, bool, error) {
	// Descend to the leaf covering key.
	if root == 0 {
		return value{}, false, nil
	}
	n, err := p.root(root)
	for err == nil && !n.leaf {
		n, err = p.child(n, childIndex(n, key))
	}
	if err != nil {
		return value{}, false, err
	}

	// Find key in the leaf.
	i, ok := n.search(key)
	if !ok {
		return value{}, false, nil
	}
	return n.vals[i], true, nil
}

// frame is one page on a cursor's path.
type frame struct {
	// n is the page.
	n *node
	// i is the position in the page.
	i int
}

// cursor walks the leaf entries of a tree in order.
type cursor struct {
	// p reads pages.
	p pager
	// root is the root page.
	root uint64
	// path holds the pages from the root to the current leaf.
	path []frame
	// err is the first read error.
	err error
}

// valid reports whether the cursor is on an entry.
func (c *cursor) valid() bool {
	if c.err != nil || len(c.path) == 0 {
		return false
	}
	f := c.path[len(c.path)-1]
	return f.i >= 0 && f.i < len(f.n.keys)
}

// key returns the current key.
func (c *cursor) key() []byte {
	f := c.path[len(c.path)-1]
	return f.n.keys[f.i]
}

// value returns the current value.
func (c *cursor) value() value {
	f := c.path[len(c.path)-1]
	return f.n.vals[f.i]
}

// descend pushes pages from page down to a leaf, taking the first child of
// each inner page when first is set and the last otherwise. With key set it
// takes the child covering key instead.
func (c *cursor) descend(page uint64, key []byte, first bool) {
	for {
		n, err := c.p.node(page)
		if err != nil {
			c.err = err
			return
		}
		i := 0
		switch {
		case key != nil && !n.leaf:
			i = childIndex(n, key)
		case !first:
			i = len(n.keys) - 1
		}
		c.path = append(c.path, frame{n: n, i: i})
		if n.leaf {
			return
		}
		page = n.kids[i]
	}
}

// seek positions the cursor at the first key at or after key, or with
// reverse set, the last key at or before it. A nil key selects the first or
// last entry.
func (c *cursor) seek(key []byte, reverse bool) {
	// Descend to the leaf covering key, or to the first or last leaf.
	c.path = c.path[:0]
	if c.root == 0 {
		return
	}
	c.descend(c.root, key, !reverse)
	if c.err != nil || key == nil {
		return
	}

	// Position in the leaf, stepping to a neighbor leaf when key falls
	// outside it.
	f := &c.path[len(c.path)-1]
	i, found := f.n.search(key)
	f.i = i
	if !reverse && i == len(f.n.keys) {
		f.i--
		c.step(false)
	}
	if reverse && !found {
		f.i--
		if f.i < 0 {
			f.i = 0
			c.step(true)
		}
	}
}

// step moves to the next entry, or the previous one when reverse is set.
func (c *cursor) step(reverse bool) {
	// Move within the leaf.
	d := 1
	if reverse {
		d = -1
	}
	f := &c.path[len(c.path)-1]
	f.i += d
	if f.i >= 0 && f.i < len(f.n.keys) {
		return
	}

	// Climb to the nearest page with a sibling in this direction.
	for len(c.path) > 1 {
		c.path = c.path[:len(c.path)-1]
		p := &c.path[len(c.path)-1]
		p.i += d
		if p.i >= 0 && p.i < len(p.n.kids) {
			c.descend(p.n.kids[p.i], nil, !reverse)
			return
		}
	}
	c.path = c.path[:0]
}

// change is one key's change applied at a checkpoint.
type change struct {
	// key is the key.
	key []byte
	// del deletes the key.
	del bool
	// val is the value a set stores.
	val value
}

// builder rewrites the pages a checkpoint changes. New pages hold their
// children as nodes until pages are assigned.
type builder struct {
	// p reads existing pages.
	p pager
	// freed lists the replaced pages.
	freed []uint64
	// sub holds the new children of each new inner page, nil where the
	// child is an existing page.
	sub map[*node][]*node
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

// build applies sorted changes to the tree at root and returns the new root.
func (b *builder) build(root uint64, changes []change) (*child, error) {
	// Rewrite the changed leaves and the pages above them.
	b.sub = make(map[*node][]*node)
	var kids []child
	var err error
	if root == 0 {
		kids = b.packLeaves(mergeLeaf(nil, nil, changes))
	} else {
		kids, err = b.apply(root, nil, changes)
	}
	if err != nil {
		return nil, err
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
		} else {
			top = child{page: top.n.kids[0]}
		}
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
		if kids[i].n == nil || fill(kids[i].n) >= pageRoom/2 {
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

// fits reports whether the entries of two children fit in one page.
func (b *builder) fits(l, r child) (bool, error) {
	total := 0
	for _, c := range []child{l, r} {
		n := c.n
		if n == nil {
			var err error
			if n, err = b.p.node(c.page); err != nil {
				return false, err
			}
		}
		total += fill(n)
	}
	return total <= pageRoom, nil
}

// merge repacks the entries of two adjacent children of one level into new
// pages, replacing any existing page among them.
func (b *builder) merge(l, r child) ([]child, error) {
	// Load existing pages, which the new ones replace.
	var nodes [2]*node
	for i, c := range []child{l, r} {
		nodes[i] = c.n
		if c.n == nil {
			n, err := b.p.node(c.page)
			if err != nil {
				return nil, err
			}
			nodes[i] = n
			b.freed = append(b.freed, c.page)
		}
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
				c.low = []child{l, r}[i].low
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

// fill returns the bytes the entries of n occupy.
func fill(n *node) int {
	total := 0
	for i, k := range n.keys {
		if n.leaf {
			total += leafEntrySize(k, n.vals[i])
		} else {
			total += innerEntrySize(k)
		}
	}
	return total
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
// top, children first, and returns their encodings and the root page.
func (b *builder) place(top *child, first uint64) ([]uint64, []*node, []byte) {
	// Number the new pages in post order.
	if top.n == nil {
		return nil, nil, nil
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

	// Resolve new children to their pages and encode.
	nums := make([]uint64, len(order))
	buf := make([]byte, 0, len(order)*pageSize)
	for i, n := range order {
		n.heads = make([]uint64, len(n.keys))
		for j, k := range n.keys {
			n.heads[j] = head(k)
		}
		if !n.leaf {
			n.inner = make([]atomic.Pointer[node], len(n.kids))
		}
		for j, s := range b.sub[n] {
			if s != nil {
				n.kids[j] = pages[s]
			}
			if s != nil && !s.leaf {
				n.inner[j].Store(s)
			}
		}
		nums[i] = pages[n]
		buf = append(buf, n.encode()...)
	}
	return nums, order, buf
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
