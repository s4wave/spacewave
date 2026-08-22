package plugin_host_scheduler

import (
	"time"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// armStartupWaitBudget arms the one-shot startup wait budget deadline.
//
// The deadline fires at most once per plugin instance boot. When it fires
// before initial capability registration completes, the budget-exhausted
// dimension is published on the existing plugin load state container. Plugin
// execution retries are not canceled or limited by the budget.
func (t *pluginInstance) armStartupWaitBudget(budget time.Duration) {
	timer := time.AfterFunc(budget, func() {
		t.markStartupWaitBudgetExhausted(budget)
	})
	t.startupWaitBudgetTimer.Store(timer)
}

// markStartupWaitBudgetExhausted publishes the budget-exhausted dimension on
// the load state. It no-ops when initial capability registration already
// completed, so a deadline racing completion cannot regress the projection.
func (t *pluginInstance) markStartupWaitBudgetExhausted(budget time.Duration) {
	state := t.updatePluginLoadState(func(current bldr_plugin.PluginLoadState) bldr_plugin.PluginLoadState {
		if current.GetInitialCapabilityRegistrationState() == bldr_plugin.InitialCapabilityRegistrationComplete {
			return current
		}
		return current.WithStartupBudgetExhausted()
	})
	if !state.GetStartupBudgetExhausted() {
		return
	}
	t.le.WithField("startup_wait_budget", budget.String()).Warn("plugin initial capability registration exceeded the startup wait budget")
}

// stopStartupWaitBudget disarms the startup wait budget deadline if armed.
func (t *pluginInstance) stopStartupWaitBudget() {
	if timer := t.startupWaitBudgetTimer.Load(); timer != nil {
		timer.Stop()
	}
}
