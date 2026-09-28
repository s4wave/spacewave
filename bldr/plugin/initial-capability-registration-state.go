package bldr_plugin

// InitialCapabilityRegistrationState reports the terminal state of a plugin
// instance's startup capability-registration pass.
type InitialCapabilityRegistrationState uint8

const (
	// InitialCapabilityRegistrationPending indicates the startup pass is running.
	InitialCapabilityRegistrationPending InitialCapabilityRegistrationState = iota
	// InitialCapabilityRegistrationComplete indicates the startup pass completed.
	InitialCapabilityRegistrationComplete
	// InitialCapabilityRegistrationFailed indicates the plugin instance ended
	// before completing the startup pass.
	InitialCapabilityRegistrationFailed
)
