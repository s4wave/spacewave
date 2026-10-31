package s4db

import (
	"context"
	"time"
)

// usageEvery is how often unlock samples the space for LastUsage. The sample
// scans the free runs under the writer lock, so it runs rarely.
const usageEvery = 10 * time.Second

// Usage describes how the file's pages are used and how the index cache
// serves them.
type Usage struct {
	// FileBytes is the file length.
	FileBytes int64
	// FreeBytes is the length of the free runs. They are punched in batches,
	// so they take little disk.
	FreeBytes int64
	// ValueBytes is the total length of the live values stored outside the
	// index.
	ValueBytes int64
	// ValuePageBytes is the length of the pages holding live values. The
	// difference from ValueBytes is the dead space inside them.
	ValuePageBytes int64
	// IndexKeys is the number of keys in the checkpoint's index.
	IndexKeys int64
	// IndexBytes is the length of the checkpoint's index pages.
	IndexBytes int64
	// LogBytes is the length of the log chunks.
	LogBytes int64
	// PendingBytes is the length of the freed runs a snapshot may still
	// read.
	PendingBytes int64
	// UnpunchedBytes is the length of the runs freed since the last hole
	// punch, which still take disk.
	UnpunchedBytes int64
	// ReservedBytes is the length of the runs this process's checkpoint is
	// writing into.
	ReservedBytes int64
	// CacheBytes is the cost of the decoded index pages the cache holds,
	// bounded by Options.CacheBytes.
	CacheBytes int64
	// PageReads counts the index pages read from the file since open. Each
	// is a cache miss, so its rate against CacheBytes shows whether the
	// cache holds the working set.
	PageReads int64
}

// sampledUsage is a Usage and when the writer took it.
type sampledUsage struct {
	u  Usage
	at time.Time
}

// Usage reports how the file's pages are used as of the latest commit. It
// takes the writer lock.
func (db *DB) Usage(ctx context.Context) (Usage, error) {
	// Read the space under the writer lock so it is current and stable.
	if err := db.w.lock(ctx); err != nil {
		return Usage{}, err
	}
	u := db.w.spaceUsage()
	db.w.unlock(nil)

	// Read the cache counters, which need no lock.
	db.cacheUsage(&u)
	return u, nil
}

// LastUsage reports the file's pages as a commit last sampled them, at most
// usageEvery before the latest commit, with when, and false before the first
// sample. It never waits for the writer lock, so a metrics scrape can call it
// while a slow commit holds the lock.
func (db *DB) LastUsage() (Usage, time.Time, bool) {
	// Load the latest sample and add the live cache counters.
	s := db.w.usage.Load()
	if s == nil {
		return Usage{}, time.Time{}, false
	}
	u := s.u
	db.cacheUsage(&u)
	return u, s.at, true
}

// cacheUsage fills the cache counters of u, which need no lock.
func (db *DB) cacheUsage(u *Usage) {
	u.CacheBytes = int64(db.p.cache.size())
	u.PageReads = db.p.reads.Load()
}

// spaceUsage reports the space and the checkpoint's index. The caller holds
// the writer lock.
func (w *writer) spaceUsage() Usage {
	// Read the free space, the runs awaiting a punch, the reserved runs and
	// the published index.
	u := w.sp.usage()
	u.UnpunchedBytes = pageOff(w.sp.releasedPages)
	for _, r := range w.sp.reserved {
		u.ReservedBytes += pageOff(r.n)
	}
	if st := w.db.cur.Load(); st != nil {
		u.IndexKeys = int64(st.sb.count) // #nosec G115 -- key counts fit in int64.
		u.IndexBytes = pageOff(st.sb.treePages)
	}
	return u
}

// sampleUsage records the space for LastUsage when usageEvery passed since
// the last sample. The caller holds the writer lock after a step that left
// the space consistent.
func (w *writer) sampleUsage() {
	now := time.Now()
	if w.sp == nil || now.Sub(w.usageAt) < usageEvery {
		return
	}
	w.usageAt = now
	w.usage.Store(&sampledUsage{u: w.spaceUsage(), at: now})
}

// OldestRead returns the commit and checkpoint of the oldest state this
// handle's snapshots still read. A freed run stays pending, taking disk, until
// no snapshot reads a state from before it was freed, so the distance from
// Seq shows how far a long snapshot holds space back.
func (db *DB) OldestRead() (seq, ckpt uint64) {
	p := db.localPin()
	return p.seq, p.ckpt
}
