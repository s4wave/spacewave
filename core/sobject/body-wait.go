package sobject

// BodyWait is the error replay returns while it cannot go on for a reason
// that is a fact about this device, not about the operation, such as a type no
// handler here resolves yet, or a block it lacks.
//
// Body.Apply returns it at that operation. The driver ends the step there,
// shows Wait in the health, and steps again once Wake is closed, without
// holding the body, so judging and installing checkpoints go on meanwhile. A
// wait with no Wake, such as a gap, steps again after the driver's backoff.
// A writer refused while replay waits gets a *WriteWait instead, so every
// BodyWait carries the replay wait it stands for.
type BodyWait struct {
	// Wait is what the health shows; never nil. The driver sets its
	// CheckpointHeight and Position.
	Wait *SOReplayWait
	// Wake is closed once the body may apply the operation. Nil when only a
	// retry after a backoff can tell.
	Wake <-chan struct{}
	// Err is the failure the wait stands for, or nil.
	Err error
}

// Error returns the error message.
func (w *BodyWait) Error() string {
	switch w.Wait.GetState().(type) {
	case *SOReplayWait_Handler, *SOReplayWait_Approval:
		return "replay waits for a handler of operation type " + w.Wait.GetOpTypeId()
	}
	if w.Err != nil {
		return ErrReplayWaits.Error() + ": " + w.Err.Error()
	}
	return ErrReplayWaits.Error()
}

// Is matches ErrReplayWaits.
func (w *BodyWait) Is(target error) bool {
	return target == ErrReplayWaits
}

// Unwrap returns the failure the wait stands for, or nil.
func (w *BodyWait) Unwrap() error {
	return w.Err
}
