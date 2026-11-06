package packahead

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
}
