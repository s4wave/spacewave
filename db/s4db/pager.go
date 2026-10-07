package s4db

import (
	"bytes"
	"context"
	"io"
	"slices"

	"github.com/pkg/errors"
)

// warmRun bounds the pages one warm-up read covers.
const warmRun = 256

// pager reads index pages through the cache, and values.
type pager struct {
	// r reads the database file.
	r io.ReaderAt
	// cache holds decoded index pages.
	cache *cache
}

// node returns a decoded index page.
func (p *pager) node(page uint64) (*node, error) {
	// Serve a cached page.
	if n := p.cache.get(page); n != nil {
		return n, nil
	}

	// Read, decode, and cache it.
	b := make([]byte, pageSize)
	if _, err := p.r.ReadAt(b, pageOff(page)); err != nil {
		return nil, err
	}
	n, err := decodeNode(b)
	if err != nil {
		return nil, errors.Wrapf(err, "page %d", page)
	}
	p.cache.put(page, n)
	return n, nil
}

// root returns the decoded root page, keeping the last one outside the
// shards.
func (p *pager) root(page uint64) (*node, error) {
	if r := p.cache.root.Load(); r != nil && r.page == page {
		return r.n, nil
	}
	n, err := p.node(page)
	if err == nil {
		p.cache.root.Store(&cacheSlot{page: page, n: n})
	}
	return n, err
}

// child returns child i of inner page n, linking decoded inner children
// into n so later lookups skip the cache.
func (p *pager) child(n *node, i int) (*node, error) {
	if c := n.inner[i].Load(); c != nil {
		return c, nil
	}
	c, err := p.node(n.kid(i))
	if err == nil && !c.leaf {
		n.inner[i].Store(c)
	}
	return c, err
}

// get looks key up in the tree at root.
func (p *pager) get(root uint64, key []byte) (value, bool, error) {
	// Descend to the leaf covering key.
	if root == 0 {
		return value{}, false, nil
	}
	n, err := p.root(root)
	for err == nil && !n.leaf {
		n, err = p.child(n, n.childIndex(key))
	}
	if err != nil {
		return value{}, false, err
	}

	// Find key in the leaf.
	i, ok := n.search(key)
	if !ok {
		return value{}, false, nil
	}
	return n.val(i), true, nil
}

// readValue returns a stored value, checking packed bytes against their
// checksum.
func (p *pager) readValue(v value) ([]byte, error) {
	// An inline value is in the index.
	if !v.isRef {
		return v.inline, nil
	}

	// Read and check packed bytes.
	b := make([]byte, v.ref.n)
	if _, err := p.r.ReadAt(b, fileOff(v.ref.off)); err != nil {
		return nil, err
	}
	if checksum(b) != v.ref.crc {
		return nil, errors.Wrapf(ErrCorrupt, "value checksum mismatch at %d", v.ref.off)
	}
	return b, nil
}

// warm reads the tree at root into the cache, one level at a time from the
// root, until a level would exceed budget bytes or ctx ends. The caller
// holds a snapshot of the tree so no page it reads is reused.
func (p *pager) warm(ctx context.Context, root uint64, budget int) error {
	level := []uint64{root}
	for root != 0 && len(level) != 0 && len(level)*pageSize <= budget {
		// Read the level.
		nodes, err := p.warmLevel(ctx, level)
		if err != nil {
			return err
		}

		// Charge it and collect the next level from inner pages.
		var next []uint64
		for _, n := range nodes {
			budget -= n.cost
			if !n.leaf {
				for i := range n.count() {
					next = append(next, n.kid(i))
				}
			}
		}
		level = next
	}
	return nil
}

// warmLevel reads pages into the cache in runs of adjacent pages and returns
// them decoded.
func (p *pager) warmLevel(ctx context.Context, pages []uint64) ([]*node, error) {
	// Order the pages by position in the file.
	sorted := slices.Clone(pages)
	slices.Sort(sorted)
	nodes := make([]*node, 0, len(sorted))

	// Read each run of adjacent pages at once.
	for len(sorted) != 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := 1
		for n < len(sorted) && n < warmRun && sorted[n] == sorted[n-1]+1 {
			n++
		}
		b := make([]byte, n*pageSize)
		if _, err := p.r.ReadAt(b, pageOff(sorted[0])); err != nil {
			return nil, err
		}

		// Decode each page from its own copy, so an evicted page does not
		// keep the run alive.
		for i := range n {
			nd, err := decodeNode(bytes.Clone(b[i*pageSize : (i+1)*pageSize]))
			if err != nil {
				return nil, err
			}
			p.cache.put(sorted[i], nd)
			nodes = append(nodes, nd)
		}
		sorted = sorted[n:]
	}
	return nodes, nil
}
