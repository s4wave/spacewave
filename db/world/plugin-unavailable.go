package world

// PluginUnavailableError reports that the plugin applying a World operation
// could not be reached or could not run it: its load failed or did not finish
// within its deadline, or reaching, attaching to or calling its handler failed
// with anything other than the handler's typed reply. The failure may pass, so
// a replay waits at the operation rather than refusing it, and every replica
// decides the position alike. A caller that cancelled gets its own error
// instead.
//
// Unless Sent, the handler was never asked to apply, so nothing it would write
// was written. When Sent, the handler may have written before the failure;
// those writes are gone only where the failed operation's transaction is
// discarded (AppliesAtomically).
type PluginUnavailableError struct {
	// PluginID is the plugin that could not be reached.
	PluginID string
	// OperationTypeID is the operation type it was to apply.
	OperationTypeID string
	// Sent reports that the handler was asked to apply before the failure.
	Sent bool
	// Err is why the plugin was unavailable: the load deadline, the load's own
	// error, or the error from reaching or calling the handler.
	Err error
}

// Error returns the unavailability explanation.
func (e *PluginUnavailableError) Error() string {
	return "plugin " + e.PluginID + " for operation " + e.OperationTypeID + " is unavailable: " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *PluginUnavailableError) Unwrap() error {
	return e.Err
}
