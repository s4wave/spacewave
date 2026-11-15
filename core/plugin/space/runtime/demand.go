package plugin_space_runtime

import (
	"slices"
)

// PluginDemand is one holder's selection of the Space plugins the runtime
// keeps running. The runtime runs the union of its demands: a contents mount
// demands every plugin the Space lists, and a background hold demands only its
// confirmed plugins. Dependencies that a running plugin loads still start on
// demand.
type PluginDemand struct {
	// c is the runtime that counts the demand.
	c *Controller
	// all selects every plugin the Space lists.
	all bool
	// replay selects its plugins even when the Space no longer lists them,
	// for a replay that waits at an operation they handle.
	replay bool
	// pluginIDs is the sorted selection when all is false. Guarded by c.bcast.
	pluginIDs []string
}

// DemandAllPlugins demands every plugin the Space lists until Release.
func (c *Controller) DemandAllPlugins() *PluginDemand {
	d := &PluginDemand{c: c, all: true}
	c.updateDemands(func() {
		c.demands[d] = struct{}{}
	})
	return d
}

// DemandPlugins demands the listed plugins until Release.
func (c *Controller) DemandPlugins(pluginIDs []string) *PluginDemand {
	d := &PluginDemand{c: c, pluginIDs: canonicalPluginIDs(pluginIDs)}
	c.updateDemands(func() {
		c.demands[d] = struct{}{}
	})
	return d
}

// DemandReplayPlugins demands the listed plugins until Release, including
// ones the Space no longer lists: the plugins a World before an operation
// that replay waits at lists. Each still starts only once the host's
// scheduler admits its manifest root.
func (c *Controller) DemandReplayPlugins(pluginIDs []string) *PluginDemand {
	d := &PluginDemand{c: c, replay: true, pluginIDs: canonicalPluginIDs(pluginIDs)}
	c.updateDemands(func() {
		c.demands[d] = struct{}{}
	})
	return d
}

// SetPluginIDs replaces the listed plugins of a DemandPlugins demand. Plugins
// that stay listed keep running.
func (d *PluginDemand) SetPluginIDs(pluginIDs []string) {
	pluginIDs = canonicalPluginIDs(pluginIDs)
	d.c.updateDemands(func() {
		d.pluginIDs = pluginIDs
	})
}

// Release withdraws the demand. Call once.
func (d *PluginDemand) Release() {
	d.c.updateDemands(func() {
		delete(d.c.demands, d)
	})
}

// IsPluginDemanded reports whether any holder demands pluginID.
func (c *Controller) IsPluginDemanded(pluginID string) bool {
	var demanded bool
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for d := range c.demands {
			if d.all || slices.Contains(d.pluginIDs, pluginID) {
				demanded = true
				return
			}
		}
	})
	return demanded
}

// GetReplayPluginIDs returns the sorted plugins replay demands, which load
// even when the Space no longer lists them.
func (c *Controller) GetReplayPluginIDs() []string {
	var ids []string
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for d := range c.demands {
			if d.replay {
				ids = append(ids, d.pluginIDs...)
			}
		}
	})
	return canonicalPluginIDs(ids)
}

// updateDemands applies change under the lock and wakes the running
// generation to reconcile its plugin loads.
func (c *Controller) updateDemands(change func()) {
	var gen *Generation
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		change()
		gen = c.gen
	})
	if gen != nil {
		gen.GetSpaceController().NotifyChanged()
	}
}
