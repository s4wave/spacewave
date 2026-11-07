package packahead

import "time"

// PackInfo describes a finalized local pack.
type PackInfo struct {
	// Seq orders the pack among the Log's packs, oldest first.
	Seq uint64
	// ID names the pack.
	ID string
	// Path is the pack's file.
	Path string
	// Size is the pack's length, its index included.
	Size int64
	// Blocks is the pack's value count.
	Blocks int
	// BloomFilter is the pack's serialized bloom filter of its keys.
	BloomFilter []byte
	// FirstAt is when the pack's first batch was written, or zero for a pack
	// loaded at Open.
	FirstAt time.Time
	// Uploaded is set once MarkUploaded recorded that the remote holds the
	// pack. It is not stored: after Open every pack starts not uploaded.
	Uploaded bool
}
