package s4db

// oentry is one change in the overlay.
type oentry struct {
	// key is the key.
	key []byte
	// del marks a deleted key.
	del bool
	// val is the stored value.
	val value
	// seq is the commit that made the change.
	seq uint64
}

// size estimates the bytes an overlay entry adds to a checkpoint.
func (e *oentry) size() int64 {
	return int64(leafEntrySize(e.key, e.val))
}
