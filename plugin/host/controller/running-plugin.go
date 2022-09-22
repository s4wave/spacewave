package plugin_host_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/util/keyed"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/pkg/errors"
)

// runningPlugin manages a running plugin instance
type runningPlugin struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// manifest is the plugin manifest
	manifest *plugin.PluginManifest
}

// newRunningPlugin constructs a new running plugin routine.
func (c *Controller) newRunningPlugin(key string) (keyed.Routine, *runningPlugin) {
	// NOTE: rmtx is locked when calling SetKey
	manifest := c.pluginManifests[key]
	tr := &runningPlugin{
		c:        c,
		pluginID: key,
		manifest: manifest,
	}
	return tr.execute, tr
}

// execute executes the plugin.
func (t *runningPlugin) execute(ctx context.Context) error {
	pluginID, le := t.pluginID, t.c.le
	manifest := t.manifest

	if manifest.GetPluginId() == "" {
		le.Debugf("fetching plugin manifest: %s", pluginID)

		// fetch the manifest for this plugin
		// wait until the plugin has been fetched
		res, err := plugin_host.ExFetchPlugin(ctx, t.c.bus, pluginID, false)
		if err != nil {
			return err
		}
		pluginManifestRef := res.GetPluginManifest()
		if err := pluginManifestRef.Validate(); err != nil {
			return errors.Wrap(err, "fetch plugin returned invalid manifest ref")
		}
		if pluginManifestRef.GetEmpty() {
			return errors.New("fetch plugin returned empty manifest ref")
		}

		// build world state handle
		ws, wsRel := t.c.buildWorldState(ctx)
		defer wsRel()

		// validate plugin manifest
		var pluginManifest *plugin.PluginManifest
		err = ws.AccessWorldState(ctx, pluginManifestRef, func(bls *bucket_lookup.Cursor) error {
			_, bcs := bls.BuildTransaction(nil)
			var err error
			pluginManifest, err = plugin.UnmarshalPluginManifest(bcs)
			return err
		})
		if err == nil {
			if pluginManifest.GetPluginId() != pluginID {
				return errors.Errorf(
					"tried to fetch plugin %s but returned manifest for %s",
					pluginID,
					pluginManifest.GetPluginId(),
				)
			}
			err = pluginManifest.Validate()
		}
		if err != nil {
			return err
		}

		// submit operation to update + link plugin manifest
		le.Debug("storing fetched plugin manifest")
		_, _, err = ws.ApplyWorldOp(plugin_host.NewUpdatePluginManifestOp(
			t.c.objKey,
			pluginID,
			pluginManifestRef,
		), t.c.peerID)
		if err != nil {
			return err
		}

		// expect that we will be reset by the changing plugin manifest
		return nil
	}

	le.Debugf("starting plugin: %s", pluginID)
	return errors.New("TODO execute plugin: " + pluginID)
}
