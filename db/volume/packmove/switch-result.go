package packmove

import volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"

// SwitchResult counts the work of a Switch.
type SwitchResult struct {
	// Copy is the work of the copy: keys, bytes and graph edges written, the
	// packed blocks and old write-back records left out.
	Copy volume_kvtx.CopyProgress
	// Outside is the number of blocks outside the World, whose values the copy
	// kept without markers.
	Outside uint64
	// OutsideBytes is the size of their values.
	OutsideBytes uint64
	// Unpacked is the number of World blocks the pack log did not commit, which
	// the copy kept with their values and an upload marker each.
	Unpacked uint64
	// UnpackedBytes is the size of their data.
	UnpackedBytes uint64
	// Bad is the number of blocks the log recorded as failing their hash. The
	// copy kept them without markers, so they upload nowhere.
	Bad uint64
}
