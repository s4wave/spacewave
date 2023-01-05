package plugin_host_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
	volume_rpc_server "github.com/aperturerobotics/hydra/volume/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
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
	ws, wsRel := t.c.buildWorldState(ctx)
	defer wsRel()

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

	le.Infof("starting plugin with manifest: %s", pluginManifest.manifestRef.MarshalString())
	return ws.AccessWorldState(ctx, pluginManifest.manifestRef, func(bls *bucket_lookup.Cursor) error {
		// build unixfs_block_fs backed by the fs
		bls.SetRootRef(manifest.GetDistFsRef())
		writer := unixfs_block_fs.NewFSWriter()
		fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, bls, writer)
		writer.SetFS(fs)
		defer fs.Release()
		ufs := unixfs.NewFS(ctx, le, fs, nil)
		defer ufs.Release()

		// add root ref to fs
		fsh, err := ufs.AddRootReference(ctx)
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
var _ plugin_host.RunningPlugin = ((*runningPlugin)(nil))
