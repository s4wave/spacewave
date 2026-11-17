package provider_local

// PeerServedClasses counts the blocks peers served by what the placement's
// open backend knew of each, classified off the read path.
type PeerServedClasses struct {
	// Indexed counts blocks the local pack index placed in a pack.
	Indexed uint64
	// NotIndexed counts blocks the index placed nowhere.
	NotIndexed uint64
	// Unclassified counts blocks dropped from a full queue, whose lookups
	// failed or outlived their bound, or with no open backend whose loaded
	// index keeps its blocks.
	Unclassified uint64
}
