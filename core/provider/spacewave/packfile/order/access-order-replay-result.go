package order

import "github.com/s4wave/spacewave/db/block"

// AccessOrderReplayResult describes profile-guided replay output.
type AccessOrderReplayResult struct {
	// Refs is the final ordered ref list.
	Refs []*block.BlockRef
	// StaleRecord indicates manifest metadata or profiled paths did not match.
	StaleRecord bool
	// UsedEntries is the number of record entries that contributed at least one ref.
	UsedEntries int
	// MissingPaths are record paths that could not be resolved in the current manifest.
	MissingPaths []string
	// MissingRefs are resolved refs that were not present in the candidate set.
	MissingRefs []*block.BlockRef
}
