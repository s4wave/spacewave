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
	// MaintainError is the background work's last failure, or nil.
	MaintainError error
}

// GetStats returns the Log's counts.
func (l *Log) GetStats() Stats {
	var s Stats
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.Packs = len(l.packs) + 1
		for _, p := range l.packs {
			if p.state == packFinal {
				s.Finalized++
			}
			s.LocalBytes += p.written
		}
		s.LocalBytes += l.open.written
		s.Entries, s.Syncs, s.MaintainError = len(l.table), l.syncs, l.maintErr
	})
	return s
}
