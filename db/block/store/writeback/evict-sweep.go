package block_store_writeback

// This file stands for the pack sweep's record of open operations until the
// store has one. The sweep defines openEvictPass on that record instead; the
// two definitions do not build together, so adding the sweep deletes this
// file.

// openEvictPass records a bounded eviction pass as an open read operation on
// the store and returns the function that ends it. A pack sweep waits on a
// pack held out of the reads while an operation that began before the hold
// is open, so a pass that verified a pack before the sweep held it finishes
// its drops before the sweep can trash that pack.
func (s *Store) openEvictPass() func() {
	return func() {}
}
