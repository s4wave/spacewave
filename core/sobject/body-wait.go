package sobject

// BodyWait is the error replay returns while it cannot go on for a reason
// that is a fact about this device, not about the operation, such as a type no
// handler here resolves yet.
//
// Body.Apply returns it at that operation. The driver ends the step there,
// shows Wait in the health, and steps again once Wake is closed, without a
// backoff and without holding the body, so judging and installing checkpoints
// go on meanwhile.
//
// A body's writer returns it, with no Wait, while replay is not caught up; the
// health's replay state says what it waits for. Wake is then closed once
// replay is caught up again.
type BodyWait struct {
	// Wait is what the health shows. The driver sets its CheckpointHeight and
	// Position. Nil in a writer's wait.
	Wait *SOReplayWait
	// Wake is closed once the body may apply the operation, or take writes.
	Wake <-chan struct{}
}

// Error returns the error message.
func (w *BodyWait) Error() string {
	if w.Wait.GetOpTypeId() == "" {
		return ErrReplayWaits.Error()
	}
	return "replay waits for a handler of operation type " + w.Wait.GetOpTypeId()
}

// Is matches ErrReplayWaits.
func (w *BodyWait) Is(target error) bool {
	return target == ErrReplayWaits
}
