package kvtx

import "time"

// SetBackgroundBatchHold sets backgroundBatchHold for a test and returns the
// function that restores it.
func SetBackgroundBatchHold(d time.Duration) func() {
	prev := backgroundBatchHold
	backgroundBatchHold = d
	return func() { backgroundBatchHold = prev }
}
