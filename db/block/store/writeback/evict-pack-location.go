package block_store_writeback

// EvictPackLocation names the remote pack a block is placed in, by the
// local index, and the pack's recorded size. An empty ID places it nowhere.
type EvictPackLocation struct {
	// ID is the pack's id, empty for none.
	ID string
	// Size is the pack's recorded size in bytes.
	Size int64
}
