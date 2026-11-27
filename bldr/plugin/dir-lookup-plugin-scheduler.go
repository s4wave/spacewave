package bldr_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// PluginScheduler is the status view of a plugin host scheduler.
type PluginScheduler interface {
	// GetInstanceKey returns the instance key the scheduler was configured
	// with. Space runtimes use the Space engine ID. The root host uses "".
	GetInstanceKey() string
	// PluginStatus returns the current plugin status snapshot.
	PluginStatus() *PluginStatusSnapshot
	// AwaitPluginStatus waits for a snapshot accepted by valid.
	AwaitPluginStatus(ctx context.Context, valid func(*PluginStatusSnapshot) (bool, error)) (*PluginStatusSnapshot, error)
	// WaitPluginStatusChange waits until status differs from previous according
	// to the scheduler's snapshot comparison.
	WaitPluginStatusChange(ctx context.Context, previous *PluginStatusSnapshot) (*PluginStatusSnapshot, error)
}

// HostPluginScheduler is a PluginScheduler that runs host plugins: the plugins
// it starts itself, which may reach the whole host bus.
type HostPluginScheduler interface {
	PluginScheduler
	// GetHostPluginIds returns the IDs of the host plugins.
	GetHostPluginIds() []string
}

// waitHostPluginScheduler waits for the root host scheduler, the one with
// instance key "", to be added to b itself. Schedulers that a runtime forwards
// from a child bus never count: their host set is not the host bus's.
func waitHostPluginScheduler(ctx context.Context, b bus.Bus) (HostPluginScheduler, error) {
	var scheduler HostPluginScheduler
	err := b.GetControllersBroadcast().Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
		for _, ctrl := range b.GetControllers() {
			s, ok := ctrl.(HostPluginScheduler)
			if ok && s.GetInstanceKey() == "" {
				scheduler = s
				return true, nil
			}
		}
		return false, nil
	})
	return scheduler, err
}

// LookupPluginScheduler is a directive to look up the plugin host schedulers
// reachable from a bus.
//
// Every scheduler resolves it with itself on its own bus. A runtime that runs
// a scheduler on a child bus forwards the directive into that bus, and a
// plugin resolves it with its host's scheduler over the PluginHost service, so
// one lookup on a parent bus collects every scheduler below it.
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
	_ LookupPluginScheduler        = (*lookupPluginScheduler)(nil)
	_ directive.DirectiveWithEquiv = (*lookupPluginScheduler)(nil)
)
