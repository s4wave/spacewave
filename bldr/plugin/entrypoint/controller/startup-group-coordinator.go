package plugin_entrypoint_controller

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// StartupPluginReferenceSource opens readiness references for startup plugins.
type StartupPluginReferenceSource interface {
	// AddPluginReference opens a readiness reference and its release function.
	AddPluginReference(pluginID, instanceKey string) (bldr_plugin.RunningPluginRef, func())
}

// StartupGroupCoordinator publishes one ready transition after every configured
// startup plugin completes or terminates initial capability registration.
type StartupGroupCoordinator struct {
	pluginIDs []string
	source    StartupPluginReferenceSource
	readyCtr  *ccontainer.CContainer[bool]
	watchers  *keyed.Keyed[string, struct{}]

	// bcast guards terminalByPluginID.
	bcast              broadcast.Broadcast
	terminalByPluginID map[string]bool
}

// NewStartupGroupCoordinator constructs a startup group coordinator.
func NewStartupGroupCoordinator(
	pluginIDs []string,
	source StartupPluginReferenceSource,
) *StartupGroupCoordinator {
	pluginIDs = slices.Clone(pluginIDs)
	slices.Sort(pluginIDs)
	pluginIDs = slices.Compact(pluginIDs)
	c := &StartupGroupCoordinator{
		pluginIDs:          pluginIDs,
		source:             source,
		readyCtr:           ccontainer.NewCContainer(false),
		terminalByPluginID: make(map[string]bool, len(pluginIDs)),
	}
	c.watchers = keyed.NewKeyed(c.newPluginWatcher)
	return c
}

// GetReadyCtr returns the watchable startup-group readiness state.
func (c *StartupGroupCoordinator) GetReadyCtr() ccontainer.Watchable[bool] {
	return c.readyCtr
}

// IsReady reports whether the startup group reached its terminal ready state.
func (c *StartupGroupCoordinator) IsReady() bool {
	return c.readyCtr.GetValue()
}

// Start begins watching the configured startup plugins.
//
// Start is idempotent: the readiness container and keyed watcher set both
// diff internally, so repeated calls are no-ops.
func (c *StartupGroupCoordinator) Start(ctx context.Context) error {
	if len(c.pluginIDs) == 0 {
		c.readyCtr.SetValue(true)
		return nil
	}
	if c.source == nil {
		return errors.New("startup plugin reference source is required")
	}

	c.watchers.SetContext(ctx, true)
	c.watchers.SyncKeys(c.pluginIDs, false)
	return nil
}

// WaitReady waits for the startup group or context cancellation.
func (c *StartupGroupCoordinator) WaitReady(ctx context.Context) error {
	_, err := c.readyCtr.WaitValue(ctx, nil)
	return err
}

func (c *StartupGroupCoordinator) newPluginWatcher(
	pluginID string,
) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		ref, release := c.source.AddPluginReference(pluginID, "")
		if release != nil {
			defer release()
		}
		if ref == nil {
			return nil
		}

		stateCtr := ref.GetPluginLoadStateCtr()
		var current bldr_plugin.PluginLoadState
		for {
			next, err := stateCtr.WaitValueChange(ctx, current, nil)
			if err != nil {
				return nil
			}
			current = next
			switch next.GetInitialCapabilityRegistrationState() {
			case bldr_plugin.InitialCapabilityRegistrationComplete,
				bldr_plugin.InitialCapabilityRegistrationFailed:
				c.setPluginTerminal(pluginID)
				return nil
			}
			// A pending plugin that exhausted its startup wait budget stops
			// blocking the group; execution retries continue unbounded.
			if next.GetStartupBudgetExhausted() {
				c.setPluginTerminal(pluginID)
				return nil
			}
		}
	}, struct{}{}
}

func (c *StartupGroupCoordinator) setPluginTerminal(pluginID string) {
	var ready bool
	var changed bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.terminalByPluginID[pluginID] {
			return
		}
		c.terminalByPluginID[pluginID] = true
		changed = true
		ready = true
		for _, id := range c.pluginIDs {
			if !c.terminalByPluginID[id] {
				ready = false
				break
			}
		}
		broadcast()
	})
	if changed {
		c.readyCtr.SetValue(ready)
	}
}
