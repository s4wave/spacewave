package provider_local

// FallbackStatus counts how a placed store's read fallback served reads and
// how its fills fared since the store mounted.
type FallbackStatus struct {
	// PackErrors counts reads whose backend error fell through to the
	// fallback. An error is never proof that a block is absent.
	PackErrors uint64
	// Hits counts blocks only the fallback held.
	Hits uint64
	// RefsUnknown counts hits the fallback holds without their outgoing refs.
	// Such a block serves but never fills, since the pack would keep it as a
	// leaf.
	RefsUnknown uint64
	// Filled counts hits stored locally with an upload marker.
	Filled uint64
	// FillFailed counts hits whose local store failed or whose bytes did not
	// match their hash. The next read of the block tries again.
	FillFailed uint64
	// FillSkipped counts hits not stored because every fill slot was busy. The
	// next read of the block tries again.
	FillSkipped uint64
}
