package bldr_plugin

import "github.com/aperturerobotics/starpc/srpc"

// PluginLoadState atomically projects the plugin RPC client, initial
// capability-registration state, and startup wait budget state.
type PluginLoadState struct {
	// manifestRoot identifies the immutable files mounted by this execution.
	manifestRoot string
	// plugin carries the live RPC connection, including during registration.
	plugin RunningPlugin
	// registrationState describes the current execution's startup result.
	registrationState InitialCapabilityRegistrationState
	// startupError carries the execution failure to the selecting scheduler.
	startupError error
	// startupBudgetExhausted reports that the plugin instance exceeded its
	// configured startup wait budget before completing registration.
	startupBudgetExhausted bool
}

// NewPluginLoadState constructs a plugin load state.
func NewPluginLoadState(
	rpcClient srpc.Client,
	registrationState InitialCapabilityRegistrationState,
) PluginLoadState {
	state := PluginLoadState{registrationState: registrationState}
	if rpcClient != nil {
		state.plugin = NewRunningPlugin(rpcClient)
	}
	return state
}

// GetRunningPlugin returns the plugin only after initial capability registration
// completes.
func (s PluginLoadState) GetRunningPlugin() RunningPlugin {
	if s.registrationState != InitialCapabilityRegistrationComplete {
		return nil
	}
	return s.plugin
}

// GetRpcClient returns the current plugin RPC client, including during startup.
func (s PluginLoadState) GetRpcClient() srpc.Client {
	if s.plugin == nil {
		return nil
	}
	return s.plugin.GetRpcClient()
}

// GetManifestRoot returns the mounted manifest root, or empty before files are ready.
func (s PluginLoadState) GetManifestRoot() string {
	return s.manifestRoot
}

// WithManifestRoot records the immutable files served by this execution.
func (s PluginLoadState) WithManifestRoot(root string) PluginLoadState {
	s.manifestRoot = root
	return s
}

// GetInitialCapabilityRegistrationState returns the startup registration state.
func (s PluginLoadState) GetInitialCapabilityRegistrationState() InitialCapabilityRegistrationState {
	return s.registrationState
}

// WithStartupBudgetExhausted returns a copy of the state marked as having
// exceeded its startup wait budget.
func (s PluginLoadState) WithStartupBudgetExhausted() PluginLoadState {
	s.startupBudgetExhausted = true
	return s
}

// GetStartupBudgetExhausted reports whether the plugin instance exceeded its
// startup wait budget before completing initial capability registration.
func (s PluginLoadState) GetStartupBudgetExhausted() bool {
	return s.startupBudgetExhausted
}

// WithStartupError marks execution as failed and preserves its terminal cause.
func (s PluginLoadState) WithStartupError(err error) PluginLoadState {
	// Withdraw the failed execution and retain its cause.
	s.manifestRoot = ""
	s.plugin = nil
	s.registrationState = InitialCapabilityRegistrationFailed
	s.startupError = err
	return s
}

// GetStartupError returns the execution error after registration fails.
func (s PluginLoadState) GetStartupError() error {
	return s.startupError
}
