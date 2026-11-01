package filelock

import "os"

// Darwin's public fcntl.h defines OFD commands that x/sys has not generated.
// These locks belong to the open file description, not a process. In particular,
// their waits do not add process-wide POSIX deadlock-detection edges.
const (
	// cmdDescriptionSet acquires or releases a descriptor-owned byte lock.
	cmdDescriptionSet = 90
	// cmdDescriptionWait waits for a descriptor-owned byte lock.
	cmdDescriptionWait = 91
	// cmdDescriptionGet queries conflicts with a descriptor-owned byte lock.
	cmdDescriptionGet = 92
)

// LockDescription takes a descriptor-owned byte lock, optionally waiting.
// Use it for intent waits that can overlap another thread's writer lock.
func LockDescription(f *os.File, off int64, wait bool) (bool, error) {
	return lockWithCommands(f, off, wait, cmdDescriptionSet, cmdDescriptionWait)
}

// UnlockDescription releases a byte acquired with LockDescription.
func UnlockDescription(f *os.File, off int64) error {
	return unlockWithCommand(f, off, cmdDescriptionSet)
}

// HeldDescription reports a conflicting descriptor-owned byte lock.
func HeldDescription(f *os.File, off int64) (bool, error) {
	return heldWithCommand(f, off, cmdDescriptionGet)
}
