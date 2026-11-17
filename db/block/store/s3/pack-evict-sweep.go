//go:build !tinygo

package block_store_s3

// This file stands for the pack sweep's hold states until the store has a
// sweep: no pack is held, so a pack in the table is registered and nothing
// halts eviction for a sweep. The sweep defines Registered and sweepHalt from
// its hold states instead, with the same contracts; the two definitions do
// not build together, so adding the sweep deletes this file.

// Registered reports whether pack id is in the plain registered state: in the
// store's pack table and held by no quarantine or trashing, so every read,
// locate and listing sees it and no sweep step will trash it before a
// quarantine. It is false for a quarantined, trashing, removed or unknown
// pack, for a store with no index, and before the table loads, so a caller
// that drops another copy of a pack's blocks on its answer keeps them when
// unsure. It reads the table in memory under its read lock, with no
// transaction.
func (s *PackStore) Registered(id string) bool {
	// No index, no registered pack.
	if s.index == nil || id == "" {
		return false
	}

	// Read the table under its read lock.
	s.index.mtx.RLock()
	defer s.index.mtx.RUnlock()
	_, known := s.index.rows[id]
	return known
}

// sweepHalt names the pack sweep while it holds any pack, or returns "".
func (s *PackStore) sweepHalt() string {
	return ""
}
