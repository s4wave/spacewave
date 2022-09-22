package plugin_host_controller

import (
	"context"

	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	world_control "github.com/aperturerobotics/hydra/world/control"
	"github.com/sirupsen/logrus"
)

// ProcessState implements the state reconciliation loop.
func (c *Controller) ProcessState(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	obj world.ObjectState, // may be nil if not found
	rootRef *bucket.ObjectRef, rev uint64,
) (waitForChanges bool, err error) {
	objKey := c.objKey
	if obj == nil {
		le.Debug("object does not exist, waiting")
		return true, nil
	}

	// collect all connected plugin manifests
	manifestKeys, err := plugin_host.ListPluginHostPluginManifests(ctx, ws, objKey)
	if err != nil {
		le.WithError(err).Warn("unable to list plugin host manifests")
		return true, err
	}
	c.syncWatchPluginManifests(manifestKeys)

	return true, nil
}

// _ is a type assertion
var _ world_control.ObjectLoopHandler = ((*Controller)(nil)).ProcessState
