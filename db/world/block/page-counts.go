package world_block

import (
	"maps"
	"sync"
)

// PageCounts counts pages by tree and level. It is safe for concurrent use;
// the zero value is empty.
type PageCounts struct {
	// mtx guards counts.
	mtx sync.Mutex
	// counts holds the counts by key, nil until the first Add.
	counts map[PageKey]PageCount
}

// Add counts one page of the given stored length.
func (c *PageCounts) Add(p Page, size int) {
	// Add the page to its key's count.
	key := PageKey{Tree: p.Tree, Level: p.Level}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.counts == nil {
		c.counts = make(map[PageKey]PageCount)
	}
	n := c.counts[key]
	n.Pages++
	n.Bytes += int64(size)
	c.counts[key] = n
}

// Snapshot returns a copy of the counts.
func (c *PageCounts) Snapshot() map[PageKey]PageCount {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return maps.Clone(c.counts)
}

// StoredPages counts the World pages this process stored, by tree and level.
var StoredPages PageCounts
