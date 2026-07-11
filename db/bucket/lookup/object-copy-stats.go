package bucket_lookup

// ObjectCopyStats contains logical source accounting for one object copy.
// BlocksSeen and LogicalSourceBytes count each eligible source block once before
// destination deduplication; sub-blocks, empty refs, and missing blocks are not
// logical copy candidates.
type ObjectCopyStats struct {
	BlocksSeen         int64
	BlocksCopied       int64
	BlocksExisting     int64
	BlocksWritten      int64
	BlocksDeduped      int64
	SubtreesSkipped    int64
	LogicalSourceBytes int64
}
