package s4db

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
	p *pager
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
	return f.i >= 0 && f.i < f.n.count()
}

// key returns the current key.
func (c *cursor) key() []byte {
	f := c.path[len(c.path)-1]
	return f.n.key(f.i)
}

// value returns the current value.
func (c *cursor) value() value {
	f := c.path[len(c.path)-1]
	return f.n.val(f.i)
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
			i = n.childIndex(key)
		case !first:
			i = n.count() - 1
		}
		c.path = append(c.path, frame{n: n, i: i})
		if n.leaf {
			return
		}
		page = n.kid(i)
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
	if !reverse && i == f.n.count() {
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
	if f.i >= 0 && f.i < f.n.count() {
		return
	}

	// Climb to the nearest page with a sibling in this direction.
	for len(c.path) > 1 {
		c.path = c.path[:len(c.path)-1]
		p := &c.path[len(c.path)-1]
		p.i += d
		if p.i >= 0 && p.i < p.n.count() {
			c.descend(p.n.kid(p.i), nil, !reverse)
			return
		}
	}
	c.path = c.path[:0]
}
