package provider_local

// PeerServedClasses counts the blocks peers served by what the placement's
// open backend knew of each, classified off the read path.
type PeerServedClasses struct {
	// IndexedHeld counts blocks the local pack index placed and the backend's
	// packs confirmed.
	IndexedHeld uint64
	// IndexedMissing counts blocks the index placed that the packs did not
	// confirm, or whose check failed.
	IndexedMissing uint64
	// NotIndexed counts blocks the index placed nowhere.
	NotIndexed uint64
	// Unclassified counts blocks dropped from a full queue, whose lookups
	// failed or outlived their bound, or with no open indexed backend.
	Unclassified uint64
}
