package plugin_host

// StartupProtocolError reports an incompatible startup exchange between two
// executable builds. Repeating the same execution cannot repair its wire format.
type StartupProtocolError struct {
	// hostBuild identifies the executable providing the host protocol.
	hostBuild string
	// pluginBuild identifies the executable attempting registration.
	pluginBuild string
	// cause preserves the failed registration diagnostic.
	cause error
}

// NewStartupProtocolError identifies the builds on both sides of a failed startup exchange.
func NewStartupProtocolError(hostBuild, pluginBuild string, cause error) *StartupProtocolError {
	return &StartupProtocolError{hostBuild: hostBuild, pluginBuild: pluginBuild, cause: cause}
}

// Error describes the stopped generation and both executable identities.
func (e *StartupProtocolError) Error() string {
	return "plugin startup protocol mismatch; host build " + e.hostBuild +
		"; plugin build " + e.pluginBuild + "; stopped until the manifest changes: " + e.cause.Error()
}

// Unwrap returns the registration failure.
func (e *StartupProtocolError) Unwrap() error {
	return e.cause
}
