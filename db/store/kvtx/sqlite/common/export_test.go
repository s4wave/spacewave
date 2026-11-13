package common

import "time"

// SetCheckpointHook sets the hook told each step of fillWALGap, and the
// deadline of the pass that closes the gap, for the rest of the test binary.
func SetCheckpointHook(hook func(step string), gapHold time.Duration) {
	checkpointHook, walGapHold = hook, gapHold
}
