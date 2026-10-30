package block_gc

// CountMismatch is a block whose count disagrees with the graph.
type CountMismatch struct {
	// Block is the block IRI.
	Block string
	// Problem says how the count disagrees.
	Problem string
}

// String formats the mismatch for a log.
func (m CountMismatch) String() string {
	return FormatIRI(m.Block) + ": " + m.Problem
}
