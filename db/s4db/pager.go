package s4db

import (
	"bytes"
	"context"
	"io"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
)

// warmRun bounds the pages one warm-up read covers.
const warmRun = 256

// nodeReads bounds the page reads nodes keeps in flight. A drive serves
// many random reads at once in about the time of one.
const nodeReads = 32

// pager reads index pages through the cache, and values.
type pager struct {
	// r reads the database file.
	r io.ReaderAt
	// cache holds decoded index pages.
	cache *cache
	// reads counts the index pages node read from the file, each a cache
	// miss.
	reads atomic.Int64
	// valueReads counts the packed values readValue read from the file.
	valueReads atomic.Int64
}

// node returns a decoded index page.
func (p *pager) node(page uint64) (*node, error) {
	// Serve a cached page.
	if n := p.cache.get(page); n != nil {
		return n, nil
	}

	// Read, decode, and cache it.
	p.reads.Add(1)
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

// nodes returns the decoded index pages, reading those not cached
// concurrently.
func (p *pager) nodes(pages []uint64) ([]*node, error) {
	// Serve the cached pages and list the rest.
	out := make([]*node, len(pages))
	var miss []int
	for i, page := range pages {
		if out[i] = p.cache.get(page); out[i] == nil {
			miss = append(miss, i)
		}
	}

	// Read the rest together, keeping the error of each read.
	errs := make([]error, len(pages))
	slots := make(chan struct{}, nodeReads)
	var wg sync.WaitGroup
	for _, i := range miss {
		slots <- struct{}{}
		wg.Go(func() {
			out[i], errs[i] = p.node(pages[i])
			<-slots
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
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

// get looks key up in the tree at root.
func (p *pager) get(root uint64, key []byte) (value, bool, error) {
	// Descend to the leaf covering key.
	if root == 0 {
		return value{}, false, nil
	}
	n, err := p.root(root)
	for err == nil && !n.leaf {
		n, err = p.node(n.kid(n.childIndex(key)))
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

// prefetchRun is a page and the sorted keys whose paths pass through it.
type prefetchRun struct {
	page uint64
	keys [][]byte
}

// prefetch reads the pages on the paths from root to each of the sorted keys
// into the cache. It descends one level at a time and keeps the reads of a
// level in flight together, so a later lookup of any key finds every page
// cached and a cold batch costs one round of reads per level.
func (p *pager) prefetch(ctx context.Context, root uint64, keys [][]byte) error {
	if root == 0 || len(keys) == 0 {
		return nil
	}
	level := []prefetchRun{{page: root, keys: keys}}
	for len(level) != 0 {
		// Read the level.
		if err := ctx.Err(); err != nil {
			return err
		}
		pages := make([]uint64, len(level))
		for i, run := range level {
			pages[i] = run.page
		}
		nodes, err := p.nodes(pages)
		if err != nil {
			return err
		}

		// Route each run of keys to the children covering them. Sorted keys
		// reach the children in order, so adjacent keys share a run.
		var next []prefetchRun
		for i, n := range nodes {
			if n.leaf {
				continue
			}
			keys := level[i].keys
			for len(keys) != 0 {
				kid := n.childIndex(keys[0])
				j := 1
				for j < len(keys) && n.childIndex(keys[j]) == kid {
					j++
				}
				next = append(next, prefetchRun{page: n.kid(kid), keys: keys[:j]})
				keys = keys[j:]
			}
		}
		level = next
	}
	return nil
}

// readValue returns a stored value, checking packed bytes against their
// checksum.
func (p *pager) readValue(v value) ([]byte, error) {
	// An inline value is in the index.
	if !v.isRef {
		return v.inline, nil
	}

	// Read and check packed bytes.
	p.valueReads.Add(1)
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
