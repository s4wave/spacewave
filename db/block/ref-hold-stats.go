package block

// RefHoldStats describes the held table.
type RefHoldStats struct {
	// Blocks is the number of distinct held blocks.
	Blocks int
	// Slots is the number of slots allocated, held or free.
	Slots int
}
