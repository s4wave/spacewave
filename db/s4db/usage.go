package s4db

import "context"

// Usage describes how the file's chunks are used and how the index cache
// serves them.
type Usage struct {
	// FileBytes is the file length.
	FileBytes int64
	// FreeBytes is the length of the free chunks. They are punched in
	// batches, so they take little disk.
	FreeBytes int64
	// ValueBytes is the total length of the live values stored outside the
	// index.
	ValueBytes int64
	// ValueChunkBytes is the length of the value chunks. The difference from
	// ValueBytes is the dead space the cleaner reclaims.
	ValueChunkBytes int64
	// IndexChunkBytes is the length of the index chunks. The difference from
	// IndexBytes is the unused pages checkpoints fill next and the space
	// records.
	IndexChunkBytes int64
	// IndexKeys is the number of keys in the checkpoint's index.
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

// Usage reports how the file's chunks are used as of the latest commit. It
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
