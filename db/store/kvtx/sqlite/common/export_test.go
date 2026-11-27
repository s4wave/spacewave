package common

import (
	"os"
	"time"
)

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

// SetDiskless disables device flushes in stores opened after it and adds delay
// before each checkpoint. The delay excludes SQLite's actual page-cache copy.
// The returned function restores both settings.
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

// HoldReadTurns takes every read turn of s until the returned func runs.
func HoldReadTurns[T SQLiteDriverConfig](s *Store[T]) func() {
	n := cap(s.readTurns)
	for range n {
		s.readTurns <- struct{}{}
	}
	return func() {
		for range n {
			<-s.readTurns
		}
	}
}

// SyncedWAL returns the WAL w keeps open because its directory entry is
// durable, nil when it keeps none.
func (w *LocalWAL) SyncedWAL() *os.File {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	return w.synced
}

// ObserveWriteHold calls observe when t returns its writer turn, with the time
// since this call. It excludes the wait for the turn and the later flow wait.
func (t *Tx) ObserveWriteHold(observe func(time.Duration)) {
	start, release := time.Now(), t.release
	t.release = func() {
		observe(time.Since(start))
		release()
	}
}
