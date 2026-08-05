//go:build js && wasm

package store

import (
	"sync"

	opfs "github.com/s4wave/spacewave/prototypes/opfs/go-opfs"
)

// handleEntry holds a refcounted FileOps handle.
type handleEntry struct {
	handle   opfs.FileOps
	refcount int
}

// handleCache manages a refcounted cache of open FileOps handles.
type handleCache struct {
	mu      sync.Mutex
	entries map[string]*handleEntry
}

func newHandleCache() *handleCache {
	return &handleCache{entries: make(map[string]*handleEntry)}
}

// acquire increments the refcount for a file (or opens it if not cached).
// The caller must provide the FileHandle to open from if not cached.
func (c *handleCache) acquire(name string, fh *opfs.FileHandle) (opfs.FileOps, error) {
	// Lock the cache while resolving or opening the named handle.
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[name]; ok {
		e.refcount++
		return e.handle, nil
	}

	// Open and retain a new handle when the cache has no entry.
	ops, err := fh.OpenFileOps()
	if err != nil {
		return nil, err
	}
	c.entries[name] = &handleEntry{handle: ops, refcount: 1}
	return ops, nil
}

// release decrements the refcount, closing the handle when it reaches zero.
func (c *handleCache) release(name string) {
	// Lock the cache while decrementing the named handle reference.
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[name]
	if !ok {
		return
	}
	e.refcount--
	if e.refcount <= 0 {
		// Close and remove handles whose final reference was released.
		_ = e.handle.Close()
		delete(c.entries, name)
	}
}

// closeAll closes all cached handles.
func (c *handleCache) closeAll() {
	// Close every cached handle while holding the cache lock.
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, e := range c.entries {
		_ = e.handle.Close()
		delete(c.entries, name)
	}
}
