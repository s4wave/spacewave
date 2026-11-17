package block_store_writeback

// maxEvictBudgetIDLen bounds a budget id.
const maxEvictBudgetIDLen = 128

// EvictBudget is a byte budget for bounded eviction. A budget is valid with a
// non-empty ID of at most 128 bytes and non-negative Bytes.
type EvictBudget struct {
	// ID names the budget. The bytes left under an id persist; a new id
	// grants Bytes afresh.
	ID string
	// Bytes is the most bytes eviction may drop under ID.
	Bytes int64
}

// valid reports whether the budget may bound a pass.
func (b EvictBudget) valid() bool {
	return b.ID != "" && len(b.ID) <= maxEvictBudgetIDLen && b.Bytes >= 0
}
