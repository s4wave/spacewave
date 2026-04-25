package cdn_bstore

import (
	"bytes"
	"context"
	"sync"
)

// memIndexCache is an in-memory IndexCache for the anonymous CDN block store.
// The CDN is read-only and the index cache is rebuilt whenever the cached
// pointer is invalidated so a durable kvtx-backed store is unnecessary.
type memIndexCache struct {
	mtx     sync.Mutex
	entries map[string][]byte
}

// newMemIndexCache constructs an empty in-memory IndexCache.
func newMemIndexCache() *memIndexCache {
	return &memIndexCache{entries: make(map[string][]byte)}
}

// Get returns cached raw index-tail bytes for a packfile.
func (c *memIndexCache) Get(_ context.Context, packID string) ([]byte, bool, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	entries, ok := c.entries[packID]
	return bytes.Clone(entries), ok, nil
}

// Set stores raw index-tail bytes for a packfile.
func (c *memIndexCache) Set(_ context.Context, packID string, entries []byte) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.entries[packID] = bytes.Clone(entries)
	return nil
}

// reset drops every cached packfile index tail.
func (c *memIndexCache) reset() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.entries = make(map[string][]byte)
}
