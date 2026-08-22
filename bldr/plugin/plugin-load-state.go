package bldr_plugin

import "github.com/aperturerobotics/starpc/srpc"

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

// PluginLoadState atomically projects the plugin RPC client, initial
// capability-registration state, and startup wait budget state.
type PluginLoadState struct {
	plugin            RunningPlugin
	registrationState InitialCapabilityRegistrationState
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
