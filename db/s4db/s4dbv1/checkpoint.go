package s4dbv1

// Checkpoint describes the checkpoint a Reader reads from.
type Checkpoint struct {
	// Seq is the last commit the tree holds.
	Seq uint64
	// Keys is the key count at Seq.
	Keys uint64
	// TreePages is the pages the tree takes.
	TreePages uint64
	// LogRunBytes is the length of the page run the log after the checkpoint
	// starts in: the most a replay reads before it follows a link.
	LogRunBytes uint64
}
