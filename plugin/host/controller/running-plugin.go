package plugin_host_controller

import (
	"context"

	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_block_fs "github.com/aperturerobotics/hydra/unixfs/block/fs"
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
	// manifest is the plugin manifest
	manifest pluginManifestSnapshot
	// mux is the rpc mux to use for incoming calls
	mux srpc.Mux
	// rpcClientCtr contains the srpc client
	rpcClientCtr *ccontainer.CContainer[*srpc.Client]
	// lastState is the last state snapshot
	// may be nil, guarded by rmtx
	lastState *plugin_host.PluginStateSnapshot
}

// newRunningPlugin constructs a new running plugin routine.
func (c *Controller) newRunningPlugin(key string) (keyed.Routine, *runningPlugin) {
	// NOTE: rmtx is locked when calling SetKey
	manifest := c.pluginManifests[key]
	tr := &runningPlugin{
		c:            c,
		le:           c.le.WithField("plugin-id", key),
		pluginID:     key,
		manifest:     manifest,
		rpcClientCtr: ccontainer.NewCContainer[*srpc.Client](nil),
	}
	tr.mux = c.buildPluginMux(key, manifest)
	return tr.execute, tr
}

// execute executes the plugin.
func (t *runningPlugin) execute(ctx context.Context) error {
	pluginID, le := t.pluginID, t.le
	manifest := t.manifest.manifest

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

	le.Infof("starting plugin with manifest: %s", t.manifest.manifestRef.MarshalString())
	return ws.AccessWorldState(ctx, t.manifest.manifestRef, func(bls *bucket_lookup.Cursor) error {
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
		execErr := t.c.host.ExecutePlugin(ctx, pluginID, manifest.GetEntrypoint(), fsh, t.rpcInitCb)
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

// rpcInitCb is called by the plugin when the RPC client is ready.
func (t *runningPlugin) rpcInitCb(client srpc.Client) (srpc.Mux, error) {
	if client == nil {
		t.rpcClientCtr.SetValue(nil)
	} else {
		t.le.Debug("plugin rpc client is ready")
		t.rpcClientCtr.SetValue(&client)
	}

	// send rpc client to watchers
	t.c.rmtx.Lock()
	stateSnapshot := plugin_host.NewPluginStateSnapshot(t.pluginID, client)
	t.lastState = stateSnapshot
	t.c.callPluginRefCallbacks(t.pluginID, stateSnapshot)
	t.c.rmtx.Unlock()

	return t.mux, nil
}
