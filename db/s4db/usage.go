package s4db

import "context"

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
	// IndexKeys is the number of keys as of the checkpoint's last commit. A
	// checkpoint that saved only part of the overlay leaves the keys changed
	// after that commit at their older entries in its index, so the index
	// may hold a few more or fewer keys than this until the next whole
	// checkpoint.
	IndexKeys int64
	// IndexBytes is the length of the checkpoint's index pages.
	IndexBytes int64
	// LogBytes is the length of the log chunks.
	LogBytes int64
	// PendingBytes is the length of the freed runs a snapshot may still
	// read.
	PendingBytes int64
	// CacheBytes is the cost of the decoded index pages the cache holds,
	// bounded by Options.CacheBytes.
	CacheBytes int64
	// PageReads counts the index pages read from the file since open. Each
	// is a cache miss, so its rate against CacheBytes shows whether the
	// cache holds the working set.
	PageReads int64
}

// Usage reports how the file's pages are used as of the latest commit. It
// takes the writer lock.
func (db *DB) Usage(ctx context.Context) (Usage, error) {
	// Hold the writer lock so the space is current and stable.
	if err := db.w.lock(ctx); err != nil {
		return Usage{}, err
	}

	// Read the space, then the checkpoint's index.
	u := db.w.sp.usage()
	sb := db.cur.Load().sb
	u.IndexKeys = int64(sb.count) // #nosec G115 -- key counts fit in int64.
	u.IndexBytes = pageOff(sb.treePages)
	db.w.unlock(nil)

	// Read the cache counters, which need no lock.
	u.CacheBytes = int64(db.p.cache.size())
	u.PageReads = db.p.reads.Load()
	return u, nil
}
