package common

import "time"

// SetCheckpointHook sets the hook told each step of a restart round, and the
// restart's hold, for the rest of the test binary.
func SetCheckpointHook(hook func(step string, err error), hold time.Duration) {
	checkpointHook, restartHold = hook, hold
}
