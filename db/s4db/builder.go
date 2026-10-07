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
	// page is an existing page, when d is nil.
	page uint64
	// d is a new page.
	d *draft
}

// builder rewrites the pages a checkpoint changes. New pages are drafts
// holding their new children until place assigns pages.
type builder struct {
	// p reads existing pages.
	p *pager
	// freed lists the replaced pages.
	freed []uint64
}

// newBuilder returns a builder reading existing pages from p.
func newBuilder(p *pager) *builder {
	return &builder{p: p}
}

// build applies sorted changes to the tree at root and returns the new root,
// or nil when the tree is left empty.
func (b *builder) build(root uint64, changes []change) (*child, error) {
	// Rewrite the changed leaves and the pages above them.
	var kids []child
	if root == 0 {
		kids = packLeaves(mergeLeaf(nil, changes))
	} else {
		var err error
		if kids, err = b.apply(root, nil, changes); err != nil {
			return nil, err
		}
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
		kids := packLeaves(mergeLeaf(n, changes))
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
		if packed := packLeaves(leaves); len(packed) != 0 {
			packed[0].low = leavesLow
			kids = append(kids, packed...)
		}
		leaves, inRun = nil, false
	}
	for i := range n.count() {
		// Keep a child without changes.
		k, page := n.key(i), n.kid(i)
		j := len(changes)
		if i+1 < n.count() {
			j, _ = slices.BinarySearchFunc(changes, n.key(i+1), func(c change, k []byte) int { return bytes.Compare(c.key, k) })
		}
		if j == 0 {
			flush()
			kids = append(kids, child{low: k, page: page})
			continue
		}

		// Add a changed leaf's entries to the run, or rewrite an inner
		// child.
		c, err := b.p.node(page)
		if err != nil {
			return nil, err
		}
		if c.leaf {
			if !inRun {
				leavesLow, inRun = k, true
			}
			b.freed = append(b.freed, page)
			leaves = append(leaves, mergeLeaf(c, changes[:j])...)
		} else {
			flush()
			sub, err := b.apply(page, k, changes[:j])
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
	out := packInner(kids)
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

// fill returns the bytes the entries of c occupy.
func (b *builder) fill(c child) (int, error) {
	if c.d != nil {
		return c.d.fill(), nil
	}
	n, err := b.p.node(c.page)
	if err != nil {
		return 0, err
	}
	return n.fill(), nil
}

// fits reports whether the entries of two children fit in one page.
func (b *builder) fits(l, r child) (bool, error) {
	total := 0
	for _, c := range []child{l, r} {
		n, err := b.fill(c)
		if err != nil {
			return false, err
		}
		total += n
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

// mergeLeaf merges sorted changes into the entries of leaf n, which may be
// nil for an empty tree.
func mergeLeaf(n *node, changes []change) []change {
	// Interleave existing entries with the changes, which win on equal keys.
	count := 0
	if n != nil {
		count = n.count()
	}
	out := make([]change, 0, count+len(changes))
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

// packInner packs children into new inner pages.
func packInner(kids []child) []child {
	// Measure the entries.
	sizes := make([]int, len(kids))
	for i, k := range kids {
		sizes[i] = innerEntrySize(k.low)
	}

	// Fill an inner page for each split.
	var out []child
	start := 0
	for _, end := range split(sizes) {
		d := &draft{keys: make([][]byte, 0, end-start), kids: make([]uint64, 0, end-start), sub: make([]*draft, 0, end-start)}
		for _, k := range kids[start:end] {
			d.keys = append(d.keys, k.low)
			d.kids = append(d.kids, k.page)
			d.sub = append(d.sub, k.d)
		}
		out = append(out, child{low: d.keys[0], d: d})
		start = end
	}
	return out
}

// place assigns consecutive pages from first to the new pages reachable from
// top, children first, passes their encodings to write in batches, and
// returns the decoded pages in page order; the last is the root.
func place(top *child, first uint64, write func(buf []byte, page uint64) error) ([]*node, error) {
	// Number the new pages in post order.
	if top == nil || top.d == nil {
		return nil, nil
	}
	pages := make(map[*draft]uint64)
	var order []*draft
	var visit func(d *draft)
	visit = func(d *draft) {
		for _, s := range d.sub {
			if s != nil {
				visit(s)
			}
		}
		pages[d] = first + uint64(len(order))
		order = append(order, d)
	}
	visit(top.d)

	// Resolve new children to their pages, encode, and write a batch of
	// pages at a time. Each returned node is decoded from its own page, so a
	// cached node never holds the buffers of the pages it replaced.
	buf := make([]byte, 0, placeBatch*pageSize)
	at := first
	placed := make(map[*draft]*node, len(order))
	out := make([]*node, len(order))
	for i, d := range order {
		for j, s := range d.sub {
			if s != nil {
				d.kids[j] = pages[s]
			}
		}
		buf = d.encode(buf)

		// Decode the page and link its new inner children.
		n, err := decodeNode(bytes.Clone(buf[len(buf)-pageSize:]))
		if err != nil {
			return nil, err
		}
		for j, s := range d.sub {
			if s != nil && !s.leaf {
				n.inner[j].Store(placed[s])
			}
		}
		placed[d], out[i] = n, n

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
func reachable(top *child) int {
	if top == nil || top.d == nil {
		return 0
	}
	var count func(d *draft) int
	count = func(d *draft) int {
		c := 1
		for _, s := range d.sub {
			if s != nil {
				c += count(s)
			}
		}
		return c
	}
	return count(top.d)
}
