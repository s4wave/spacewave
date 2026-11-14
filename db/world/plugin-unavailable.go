package world

// PluginUnavailableError reports that the plugin applying a World operation
// could not be reached before the operation was sent to it: its load failed or
// did not finish within Deadline. It returns from preparing the operation,
// before any transaction opens, so nothing was sent to apply and nothing was
// written. A caller that cancelled the load gets its own error instead.
type PluginUnavailableError struct {
	// PluginID is the plugin that could not be reached.
	PluginID string
	// OperationTypeID is the operation type it was to apply.
	OperationTypeID string
	// Err is why the load failed: the load deadline or the load's own error.
	Err error
}

// Error returns the unavailability explanation.
func (e *PluginUnavailableError) Error() string {
	return "plugin " + e.PluginID + " for operation " + e.OperationTypeID + " is unavailable: " + e.Err.Error()
}

// Unwrap returns the load error.
func (e *PluginUnavailableError) Unwrap() error {
	return e.Err
}
