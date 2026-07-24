package plugin_entrypoint_controller

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/ccontainer"
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
	remaining atomic.Int64
	startOnce sync.Once
	readyOnce sync.Once
	startErr  error
}

// NewStartupGroupCoordinator constructs a startup group coordinator.
func NewStartupGroupCoordinator(
	pluginIDs []string,
	source StartupPluginReferenceSource,
) *StartupGroupCoordinator {
	pluginIDs = slices.Clone(pluginIDs)
	slices.Sort(pluginIDs)
	pluginIDs = slices.Compact(pluginIDs)
	return &StartupGroupCoordinator{
		pluginIDs: pluginIDs,
		source:    source,
		readyCtr:  ccontainer.NewCContainer(false),
	}
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
func (c *StartupGroupCoordinator) Start(ctx context.Context) error {
	c.startOnce.Do(func() {
		if len(c.pluginIDs) == 0 {
			c.markReady()
			return
		}
		if c.source == nil {
			c.startErr = errors.New("startup plugin reference source is required")
			return
		}

		c.remaining.Store(int64(len(c.pluginIDs)))
		for _, pluginID := range c.pluginIDs {
			ref, release := c.source.AddPluginReference(pluginID, "")
			go c.watchPlugin(ctx, ref, release)
		}
	})
	return c.startErr
}

// WaitReady waits for the startup group or context cancellation.
func (c *StartupGroupCoordinator) WaitReady(ctx context.Context) error {
	_, err := c.readyCtr.WaitValue(ctx, nil)
	return err
}

func (c *StartupGroupCoordinator) watchPlugin(
	ctx context.Context,
	ref bldr_plugin.RunningPluginRef,
	release func(),
) {
	if release != nil {
		defer release()
	}
	if ref == nil {
		return
	}

	stateCtr := ref.GetPluginLoadStateCtr()
	var current bldr_plugin.PluginLoadState
	for {
		next, err := stateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return
		}
		current = next
		switch next.GetInitialCapabilityRegistrationState() {
		case bldr_plugin.InitialCapabilityRegistrationComplete,
			bldr_plugin.InitialCapabilityRegistrationFailed:
			if c.remaining.Add(-1) == 0 {
				c.markReady()
			}
			return
		}
	}
}

func (c *StartupGroupCoordinator) markReady() {
	c.readyOnce.Do(func() {
		c.readyCtr.SetValue(true)
	})
}
