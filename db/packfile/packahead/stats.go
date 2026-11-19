package packahead

// Stats counts what a Log holds and has done.
type Stats struct {
	// Packs counts the local packs, the open one included.
	Packs int
	// Finalized counts the finalized local packs.
	Finalized int
	// LocalBytes sums the local packs' lengths.
	LocalBytes int64
	// Entries counts the values the table locates.
	Entries int
	// Syncs counts the fdatasyncs writers have run.
	Syncs uint64
	// SyncFailures counts the fdatasyncs that failed.
	SyncFailures uint64
	// Poisoned counts the local packs a failed sync poisoned, which stay
	// unfinalized until the next Open recovers them.
	Poisoned int
	// MaintainError is the background work's last failure, or nil.
	MaintainError error
}

// GetStats returns the Log's counts.
func (l *Log) GetStats() Stats {
	var s Stats
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Count the tracked packs and the open writer.
		s.Packs, s.Finalized, s.Poisoned = len(l.packs)+1, l.finals.Len(), l.poisoned
		s.LocalBytes = l.localBytes + l.open.written

		// Read the Log's counters.
		s.Entries, s.Syncs, s.SyncFailures, s.MaintainError = len(l.table), l.syncs, l.syncFailures, l.maintErr
	})
	return s
}
