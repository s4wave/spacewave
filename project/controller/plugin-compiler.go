package bldr_project_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/util/keyed"
)

// pluginBuilderTracker tracks a running plugin build controller.
type pluginBuilderTracker struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
}

// newPluginBuilderTracker constructs a new plugin build controller tracker.
func (c *Controller) newPluginBuilderTracker(key string) (keyed.Routine, *pluginBuilderTracker) {
	tr := &pluginBuilderTracker{
		c:        c,
		pluginID: key,
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *pluginBuilderTracker) execute(ctx context.Context) error {
	pluginID, le := t.pluginID, t.c.le

	le.Debugf("starting plugin build controller: %s", pluginID)
	// TODO
	return nil
}
