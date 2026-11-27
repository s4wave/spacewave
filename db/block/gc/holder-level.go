package block_gc

// HolderLevel counts the rows streamed at one distance above the requested refs.
type HolderLevel struct {
	// Rows counts rows, including a parent once per requested child its list references.
	Rows uint64
	// States counts rows by the collector's detailed front proof, even for rows outside the front.
	States [5]uint64
	// Followed counts distinct retained parent refs to inspect at the next level.
	Followed int
}
