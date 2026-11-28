package block_gc

// AdoptProgress is how far an adoption in progress has come.
type AdoptProgress struct {
	// Phase is the pass in progress.
	Phase AdoptPhase
	// Slices counts the slices this run handled, each up to sliceRecords
	// records of the pass.
	Slices uint64
	// Blocks counts the stored blocks settled so far, by every run.
	Blocks uint64
	// Queued counts the settled blocks queued at zero.
	Queued uint64
}

// AdoptWatcher is implemented by a Store that follows an adoption, which takes
// as long as the Volume is large and otherwise reports only its end.
type AdoptWatcher interface {
	// AdoptProgress is called for each slice of an adoption, inside the
	// slice's transaction, so it returns at once.
	AdoptProgress(p AdoptProgress)
}
