package block_store_writeback

import "time"

// Status is the upload state of a Store.
type Status struct {
	// Enabled reports whether writes are queued for upload.
	Enabled bool
	// Pending is the number of blocks awaiting upload.
	Pending int
	// PendingBytes is the size of the blocks awaiting upload.
	PendingBytes int64
	// Oldest is the write time of the first block awaiting upload, while
	// Pending is not zero. Every block written before it has uploaded.
	Oldest time.Time
	// Err is the last upload failure, cleared by the next successful batch.
	Err error
	// Target is the remote every block stored before it was chosen is queued
	// for, empty until a backfill completes.
	Target string
	// Policy sets when queued blocks upload and the resident budget.
	Policy Policy
	// EvictableBytes is the size of the uploaded blocks the eviction queue
	// holds. It can overcount blocks the garbage collector swept until an
	// eviction pass meets their entries.
	EvictableBytes int64
	// EvictedBytes is the size of the values eviction dropped since the store
	// opened.
	EvictedBytes int64
	// Uploaded is the number of blocks uploaded since the store opened.
	Uploaded int64
	// UploadedBytes is the size of the blocks uploaded since the store
	// opened.
	UploadedBytes int64
	// UploadReads is the bound on the local reads an upload keeps in flight,
	// which falls while the disk is loaded.
	UploadReads int
	// Refills is the number of evicted blocks read back from the remote since
	// the store opened. Each one was a read that waited on the remote.
	Refills int64
	// Copy reports the latest copy of the stored blocks to a remote.
	Copy CopyStatus
}

// ResidentBytes is the size of the store's blocks the local store holds: the
// blocks awaiting upload and the uploaded blocks not yet evicted.
func (s Status) ResidentBytes() int64 {
	return s.PendingBytes + s.EvictableBytes
}
