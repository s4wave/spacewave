package sobject

// BodyWait is the error Body.Apply returns while it cannot apply its operation
// for a reason that is a fact about this device, not about the operation, such
// as a type no handler here resolves yet. The driver ends the step there, shows
// Wait in the health, and steps again once Wake is closed, without a backoff
// and without holding the body, so judging and installing checkpoints go on
// meanwhile.
type BodyWait struct {
	// Wait is what the health shows. The driver sets its CheckpointHeight and
	// Position.
	Wait *SOReplayWait
	// Wake is closed once the body may apply the operation.
	Wake <-chan struct{}
}

// Error returns the error message.
func (w *BodyWait) Error() string {
	return "replay waits for a handler of operation type " + w.Wait.GetOpTypeId()
}
