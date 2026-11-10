package packahead

import "slices"

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
	// Poisoned counts the local packs a failed write or sync poisoned and
	// salvage has not replaced yet.
	Poisoned int
	// Salvaged counts the poisoned packs salvaged in process.
	Salvaged uint64
	// SalvageFailures counts the salvages that failed. After one, the Log
	// refuses every Append until it is opened again.
	SalvageFailures uint64
	// FailError is the error every Append returns after a failed salvage.
	FailError error
	// MaintainError is the background work's last failure, or nil.
	MaintainError error
}

// GetStats returns the Log's counts.
func (l *Log) GetStats() Stats {
	var s Stats
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Count the packs, the open one included.
		s.Packs = len(l.packs) + 1
		for _, p := range append(slices.Clone(l.packs), l.open) {
			if p.state == packFinal {
				s.Finalized++
			}
			if p.poisoned {
				s.Poisoned++
			}
			s.LocalBytes += p.written
		}

		// Read the Log's counters.
		s.Entries, s.Syncs, s.SyncFailures, s.MaintainError = len(l.table), l.syncs, l.syncFailures, l.maintErr
		s.Salvaged, s.SalvageFailures, s.FailError = l.salvaged, l.salvageFailures, l.failErr
	})
	return s
}
