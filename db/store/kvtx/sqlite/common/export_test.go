package common

import "time"

// SetCheckpointHook sets the hook told each step of fillWALGap, and the
// deadline of the pass that closes the gap, for the rest of the test binary.
func SetCheckpointHook(hook func(step string), gapHold time.Duration) {
	checkpointHook, walGapHold = hook, gapHold
}

// DefaultGapHold is the deadline of the pass that closes the gap, for tests
// to restore after SetCheckpointHook.
var DefaultGapHold = walGapHold

// FlowWaitMax and TurnHold are the flow's longest wait once its commit is
// copied and the checkpointer's back-pressure slice.
const (
	FlowWaitMax = walFlowWaitMax
	TurnHold    = walTurnHold
)

// SetDiskless keeps the device out of the stores opened after it and adds
// delay to each of their passes, standing in for a disk that copies at a
// known speed, until the returned func restores both.
func SetDiskless(delay time.Duration) (restore func()) {
	diskless, copyDelay = true, delay
	return func() { diskless, copyDelay = false, 0 }
}

// SetReadTurns gives s's snapshot reads n turns.
func SetReadTurns[T SQLiteDriverConfig](s *Store[T], n int) {
	s.readTurns = make(readTurns, n)
}

// WakeCheckpoint wakes s's checkpointer as a commit would, without one.
func WakeCheckpoint(s any) {
	s.(interface{ wakeCheckpoint() }).wakeCheckpoint()
}
