package provider_local

import (
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
)

// defaultSQLLandedBytes is the default bound of the landed blocks each
// SQL-primary publisher keeps in memory.
const defaultSQLLandedBytes = 32 << 20

// sqlLandedBytes bounds the landed blocks of the SQL-primary stores mounted
// after it is set; see SetSQLLandedBytes.
var sqlLandedBytes = func() *atomic.Int64 {
	v := new(atomic.Int64)
	v.Store(defaultSQLLandedBytes)
	return v
}()

// SetSQLLandedBytes bounds the bytes of landed blocks the publisher of each
// SQL-primary store mounted after the call keeps in memory. Zero or less turns
// the landed blocks off, so every read past the overlay reads its row.
func SetSQLLandedBytes(n int64) {
	sqlLandedBytes.Store(max(n, 0))
}

// sqlLanded keeps the newest blocks whose rows landed in MariaDB, oldest
// evicted first within a byte bound, so the first reads of a new head, such
// as the World change watch comparing it with its base, come from memory
// rather than one row at a time. A hit is marked durable, so the decoded block
// enters the shared pool on that first read.
//
// It compensates for SQL primary having no memory layer for raw blocks: the
// decoded pool admits a block only on a durable read, and the overlay drops a
// block as its row lands. A local durable store in front of MariaDB replaces
// it.
//
// Invariant: it holds only blocks whose rows sql.Store has committed; an entry
// answers the same bytes the row holds. Entries are added after the insert
// returns. Each records the removal generation taken before its insert, and a
// lookup drops an entry whose ref was removed since: once rows can be
// reclaimed, the reclaim's InvalidateRemovedBlock makes the entry miss, and
// the read falls through to the store, which finds no row. A hit marks the
// read durable at that same generation, so a removal after the lookup keeps
// the decoded block out of the pool too. A memory hit never marks a removed
// block durable.
type sqlLanded struct {
	// max is the byte bound, zero when off.
	max int64
	// mtx guards the fields below.
	mtx sync.Mutex
	// queue holds the entries oldest first from head.
	queue []*sqlLandedEntry
	head  int
	// byRef indexes the queued entries by ref.
	byRef map[string]*sqlLandedEntry
	// bytes sums the queued entries' data.
	bytes int64
	// hits and misses count the lookups past the overlay.
	hits, misses atomic.Uint64
}

// sqlLandedEntry is one landed block.
type sqlLandedEntry struct {
	key   string
	entry *block.PutBatchEntry
	// gen is the removal generation taken before the insert.
	gen uint64
}

// newSQLLanded returns landed blocks bounded by maxBytes, off at zero.
func newSQLLanded(maxBytes int64) *sqlLanded {
	return &sqlLanded{max: maxBytes, byRef: make(map[string]*sqlLandedEntry)}
}

// add keeps the landed entries, whose insert began at removal generation gen,
// evicting the oldest past the bound. A block larger than the bound is not
// kept.
func (l *sqlLanded) add(entries []*block.PutBatchEntry, gen uint64) {
	// Keep nothing when off.
	if l.max == 0 {
		return
	}

	// Queue each block not already held, then evict past the bound.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	for _, entry := range entries {
		key := entry.Ref.MarshalString()
		size := int64(len(entry.Data))
		if l.byRef[key] != nil || size > l.max {
			continue
		}
		e := &sqlLandedEntry{key: key, entry: entry, gen: gen}
		l.byRef[key] = e
		l.queue = append(l.queue, e)
		l.bytes += size
	}
	for l.bytes > l.max {
		l.evictLocked()
	}
}

// evictLocked drops the oldest entry. The caller holds mtx.
func (l *sqlLanded) evictLocked() {
	// Pop the head and unindex it.
	e := l.queue[l.head]
	l.queue[l.head] = nil
	l.head++
	if l.byRef[e.key] == e {
		delete(l.byRef, e.key)
	}
	l.bytes -= int64(len(e.entry.Data))

	// Compact once the evicted prefix is half the queue.
	if l.head*2 >= len(l.queue) {
		l.queue = append(l.queue[:0], l.queue[l.head:]...)
		l.head = 0
	}
}

// get returns the landed entry for ref with its insert's removal generation,
// or nil when it holds none or ref was removed since.
func (l *sqlLanded) get(ref *block.BlockRef) (*block.PutBatchEntry, uint64) {
	// Answer nothing when off.
	if l.max == 0 {
		return nil, 0
	}

	// Find the entry, dropping it when ref was removed since its insert. The
	// queue keeps the dropped entry until it is evicted, still counted.
	key := ref.MarshalString()
	l.mtx.Lock()
	e := l.byRef[key]
	if e != nil && block.RemovedSince(ref, e.gen) {
		delete(l.byRef, key)
		e = nil
	}
	l.mtx.Unlock()

	// Count the lookup.
	if e == nil {
		l.misses.Add(1)
		return nil, 0
	}
	l.hits.Add(1)
	return e.entry, e.gen
}

// held returns the bytes held.
func (l *sqlLanded) held() int64 {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.bytes
}
