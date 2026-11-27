package sobject

// WriteWait is the error a body's writer returns while replay waits: nothing
// was written. The health's replay state says what replay waits for.
type WriteWait struct {
	// Wake is closed once the body's state changes. The state change may end
	// the wait.
	Wake <-chan struct{}
}

// Error returns the error message.
func (w *WriteWait) Error() string {
	return ErrReplayWaits.Error()
}

// Is matches ErrReplayWaits.
func (w *WriteWait) Is(target error) bool {
	return target == ErrReplayWaits
}
