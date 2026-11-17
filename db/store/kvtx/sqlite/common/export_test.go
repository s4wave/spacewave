package common

import "time"

// SetCheckpointHook sets the hook told each step of fillWALGap, and the
// deadline of the pass that closes the gap, for the rest of the test binary.
func SetCheckpointHook(hook func(step string), gapHold time.Duration) {
	checkpointHook, walGapHold = hook, gapHold
}

// SetReadTurns gives s's snapshot reads n turns.
func SetReadTurns[T SQLiteDriverConfig](s *Store[T], n int) {
	s.readTurns = make(readTurns, n)
}

// WakeCheckpoint wakes s's checkpointer as a commit would, without one.
func WakeCheckpoint(s any) {
	s.(interface{ wakeCheckpoint() }).wakeCheckpoint()
}
