package s4db

import "testing"

// TestPageWriterCachesInnerPages checks that a checkpoint caches the inner
// pages it writes but not the leaves, and that a reused page's stale entry
// leaves the cache when its run is reserved.
func TestPageWriterCachesInnerPages(t *testing.T) {
	// Cache a stale page where the writer will place a leaf.
	c := newCache(1 << 20)
	c.put(10, &node{leaf: true, cost: pageSize})
	w := &pageWriter{
		cache:   c,
		reserve: func(uint64) ([]run, error) { return []run{{start: 10, n: 4}}, nil },
		write:   func([]byte, uint64) error { return nil },
	}

	// Add a leaf and the inner page above it.
	leaf, err := w.add(&draft{leaf: true, keys: [][]byte{[]byte("k")}, vals: []value{{inline: []byte("v")}}})
	if err != nil {
		t.Fatal(err)
	}
	inner, err := w.add(&draft{keys: [][]byte{nil}, kids: []uint64{leaf}, sub: []*draft{nil}})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.flush(); err != nil {
		t.Fatal(err)
	}

	// Only the inner page is cached.
	if c.get(leaf) != nil {
		t.Fatalf("leaf page %d is cached", leaf)
	}
	if n := c.get(inner); n == nil || n.leaf {
		t.Fatalf("inner page %d cached as %v", inner, n)
	}
}
