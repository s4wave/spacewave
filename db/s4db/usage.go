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

// Usage reports how the file's space is used.
func (db *DB) Usage(ctx context.Context) (Usage, error) {
	return Usage{}, ErrUnimplemented
}
