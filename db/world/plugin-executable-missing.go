package world

// PluginExecutableMissingError reports that the plugin build an operation is
// pinned to has no executable for any platform this device runs, so the
// operation cannot be applied here until the device's platforms change. It
// is a fact about this device, not about the operation: it never becomes the
// operation's stored rejection, and a replay waits at the operation rather
// than refusing it. It returns before the operation is sent to any plugin, so
// nothing was written.
type PluginExecutableMissingError struct {
	// PluginID is the plugin the operation is pinned to.
	PluginID string
	// OperationTypeID is the operation type it was to apply.
	OperationTypeID string
	// ManifestRoot is the pinned manifest root.
	ManifestRoot string
}

// Error returns the explanation.
func (e *PluginExecutableMissingError) Error() string {
	return "plugin " + e.PluginID + " for operation " + e.OperationTypeID + " has no build for this device"
}

// _ is a type assertion
var _ error = (*PluginExecutableMissingError)(nil)
