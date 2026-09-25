package plugin_host_scheduler

import (
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
)

// PluginScheduler is the status view of a plugin host scheduler.
type PluginScheduler interface {
	// GetInstanceKey returns the instance key the scheduler was configured
	// with. Space runtimes use the Space engine ID; the root host uses "".
	GetInstanceKey() string
	// GetPluginStatusCtr returns the scheduler's live plugin-status snapshot.
	GetPluginStatusCtr() ccontainer.Watchable[*PluginStatusSnapshot]
}

// LookupPluginScheduler is a directive to look up the plugin host schedulers
// reachable from a bus.
//
// Every scheduler resolves it with itself on its own bus. A runtime that runs
// a scheduler on a child bus forwards the directive into that bus, so one
// lookup on a parent bus collects every scheduler below it.
type LookupPluginScheduler interface {
	// Directive indicates LookupPluginScheduler is a directive.
	directive.Directive

	// IsLookupPluginScheduler marks the directive type.
	IsLookupPluginScheduler()
}

// LookupPluginSchedulerValue is the result type for LookupPluginScheduler.
// Multiple results may be pushed to the directive.
type LookupPluginSchedulerValue = PluginScheduler

// lookupPluginScheduler implements LookupPluginScheduler.
type lookupPluginScheduler struct{}

// NewLookupPluginScheduler constructs a new LookupPluginScheduler directive.
func NewLookupPluginScheduler() LookupPluginScheduler {
	return &lookupPluginScheduler{}
}

// IsLookupPluginScheduler marks the directive type.
func (d *lookupPluginScheduler) IsLookupPluginScheduler() {}

// Validate validates the directive.
func (d *lookupPluginScheduler) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *lookupPluginScheduler) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// IsEquivalent reports whether other is also a LookupPluginScheduler.
func (d *lookupPluginScheduler) IsEquivalent(other directive.Directive) bool {
	_, ok := other.(LookupPluginScheduler)
	return ok
}

// GetName returns the directive's type name.
func (d *lookupPluginScheduler) GetName() string {
	return "LookupPluginScheduler"
}

// GetDebugVals returns the directive arguments stringified.
func (d *lookupPluginScheduler) GetDebugVals() directive.DebugValues {
	return nil
}

// _ is a type assertion
var (
	_ PluginScheduler              = (*Controller)(nil)
	_ LookupPluginScheduler        = (*lookupPluginScheduler)(nil)
	_ directive.DirectiveWithEquiv = (*lookupPluginScheduler)(nil)
)
