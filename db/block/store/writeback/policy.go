package block_store_writeback

import "time"

// Policy sets when queued blocks upload and how many bytes the local store
// keeps.
type Policy struct {
	// Settle is how long a block stays queued before it uploads.
	Settle time.Duration
	// MaxPendingBytes uploads the oldest blocks before they settle while the
	// queued bytes exceed it. Zero sets no limit.
	MaxPendingBytes int64
	// ResidentHigh is the resident byte budget: past it, eviction drops the
	// uploaded blocks read least recently. Zero evicts nothing.
	ResidentHigh int64
	// ResidentLow is where eviction stops, at most ResidentHigh. Eviction can
	// drop only uploaded blocks, so queued bytes past it upload before they
	// settle.
	ResidentLow int64
	// MaxUploadReads bounds the local reads the upload keeps in flight. The
	// upload lowers its bound while reads slow, yielding the disk to the
	// store's own reads. Zero uses defaultMaxReads.
	MaxUploadReads int
	// UploadReadSlow is the least read time the upload counts as the disk
	// being loaded. Zero uses defaultReadSlow.
	UploadReadSlow time.Duration
}

// overLimit reports whether pending bytes exceed the policy's limits.
func (p Policy) overLimit(pendingBytes int64) bool {
	return p.MaxPendingBytes > 0 && pendingBytes > p.MaxPendingBytes ||
		p.ResidentHigh > 0 && pendingBytes > p.ResidentLow
}
