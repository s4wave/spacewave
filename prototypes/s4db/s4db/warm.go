//go:build darwin || linux

package s4db

import (
	"bytes"
	"slices"
)

// warmRun bounds the pages one warm-up read covers.
const warmRun = 256

// warm reads the tree of the published state into the cache, one level at a
// time from the root, reading each level's pages in file order and joining
// adjacent pages into one read. It stops when the cache is full or the
// handle closes, and holds a snapshot so no page it reads is reused.
func (db *DB) warm() {
	// Pin the published state while reading its pages.
	defer close(db.warmDone)
	st, stripe := db.acquire()
	defer db.release(st, stripe)

	// Descend while the level fits the remaining cache.
	budget := db.opts.CachePages
	level := []uint64{st.root}
	for st.root != 0 && len(level) != 0 && len(level) <= budget {
		nodes, ok := db.warmLevel(level)
		if !ok {
			return
		}
		budget -= len(level)

		// Collect the next level from inner pages.
		var next []uint64
		for _, n := range nodes {
			if !n.leaf {
				next = append(next, n.kids...)
			}
		}
		level = next
	}
}

// warmLevel reads pages into the cache in runs of adjacent pages and returns
// them decoded. It reports false when the handle closes or a read fails.
func (db *DB) warmLevel(pages []uint64) ([]*node, bool) {
	// Order the pages by position in the file.
	sorted := slices.Clone(pages)
	slices.Sort(sorted)
	nodes := make([]*node, 0, len(sorted))

	// Read each run of adjacent pages at once.
	for len(sorted) != 0 {
		n := 1
		for n < len(sorted) && n < warmRun && sorted[n] == sorted[n-1]+1 {
			n++
		}
		select {
		case <-db.closing:
			return nil, false
		default:
		}
		b := make([]byte, n*pageSize)
		if _, err := db.f.ReadAt(b, int64(sorted[0]*pageSize)); err != nil {
			return nil, false
		}

		// Decode each page from its own copy, so an evicted page does not
		// keep the run alive.
		for i := range n {
			nd, err := decodeNode(bytes.Clone(b[i*pageSize : (i+1)*pageSize]))
			if err != nil {
				return nil, false
			}
			db.cache.put(sorted[i], nd)
			nodes = append(nodes, nd)
		}
		sorted = sorted[n:]
	}
	return nodes, true
}
