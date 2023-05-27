package plugin_host_controller

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

// runningPlugin manages a running plugin instance
type runningPlugin struct {
	// c is the controller
	c *Controller
	// le is the logger
	le *logrus.Entry
	// pluginID is the plugin id
	pluginID string
	// mux is the rpc mux to use for incoming calls
	mux srpc.Mux
	// rpcClientCtr contains the srpc client
	rpcClientCtr *ccontainer.CContainer[*srpc.Client]
}

// newRunningPlugin constructs a new running plugin routine.
func (c *Controller) newRunningPlugin(key string) (keyed.Routine, *runningPlugin) {
	tr := &runningPlugin{
		c:            c,
		le:           c.le.WithField("plugin-id", key),
		pluginID:     key,
		rpcClientCtr: ccontainer.NewCContainer[*srpc.Client](nil),
	}
	return tr.execute, tr
}

// GetRpcClientCtr returns the rpc client container.
func (t *runningPlugin) GetRpcClientCtr() *ccontainer.CContainer[*srpc.Client] {
	return t.rpcClientCtr
}

// execute executes the plugin.
func (t *runningPlugin) execute(ctx context.Context) error {
	pluginID, le := t.pluginID, t.le

	// build proxy volume
	hostVol, err := t.c.hostVolumeCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}
	proxyHostVol := volume_rpc_server.NewProxyVolume(ctx, hostVol.vol, false)

	// build mux
	t.c.rmtx.Lock()
	pluginManifest := t.c.pluginManifests[pluginID]
	manifest := pluginManifest.manifest
	hostMux := t.c.buildPluginMux(pluginID, pluginManifest, proxyHostVol, hostVol.info)
	t.c.rmtx.Unlock()

	// build world state handle
	ws, err := t.c.getWorldState(ctx)
	if err != nil {
		return err
	}

	// fetch the manifest if it doesn't exist
	emptyManifest := manifest.GetMeta().GetManifestId() == ""
	if emptyManifest || t.c.conf.GetAlwaysFetchManifest() {
		ref, fetcher, _ := t.c.pluginManifestFetchers.AddKeyRef(pluginID)
		defer ref.Release()

		if emptyManifest {
			// expect that we will be reset by the changing plugin manifest
			_, err := fetcher.resultPromise.Await(ctx)
			return err
		}
	}

	le.Infof("starting plugin with manifest: %s", pluginManifest.manifestRef.MarshalString())
	return manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, pluginManifest.manifestRef, func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FS,
		assetsFS *unixfs.FS,
	) error {
		// add root ref to dist fs
		fsh, err := distFS.AddRootReference(ctx)
		if err != nil {
			return err
		}
		defer fsh.Release()

		// execute the plugin
		execErr := t.c.host.ExecutePlugin(
			ctx,
			pluginID,
			manifest.GetEntrypoint(),
			fsh,
			hostMux,
			t.updateRpcClient,
		)

		// clear the rpc client after the plugin exits
		t.updateRpcClient(nil)

		// handle if the plugin returned an error
		if execErr != nil {
			select {
			case <-ctx.Done():
				// if the context was canceled, return that error instead.
				return context.Canceled
			default:
			}
			// TODO: track this error in PluginStatus
			le.WithError(execErr).Error("plugin execution errored")
			return execErr
		}

		return nil
	})
}

// updateRpcClient is called by the plugin when the RPC client changes.
func (t *runningPlugin) updateRpcClient(client srpc.Client) error {
	_ = t.rpcClientCtr.SwapValue(func(val *srpc.Client) *srpc.Client {
		changed := ((client == nil) != (val == nil)) || (val != nil && *val != client)
		if !changed {
			return val
		}
		if client == nil {
			t.le.Debug("plugin rpc client is unset")
			return nil
		}
		t.le.Debug("plugin rpc client is ready")
		return &client
	})

	return nil
}

// _ is a type assertion
var _ plugin.RunningPlugin = ((*runningPlugin)(nil))
