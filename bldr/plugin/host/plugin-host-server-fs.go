package plugin_host

import (
	"context"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
)

// pluginHostServerFsTracker tracks a plugin fs for ongoing rpc calls for the plugin host server.
type pluginHostServerFsTracker struct {
	// s is the server
	s *PluginHostServer
	// pluginID is the plugin id
	pluginID string
	// resultPromiseCtr contains the plugin reference.
	resultPromiseCtr *promise.PromiseContainer[*pluginHostServerFsTrackerResult]
}

// pluginHostServerFsTrackerResult is the loaded result of the pluginHostServerFsTracker
type pluginHostServerFsTrackerResult struct {
	assetsUnixFSID, distUnixFSID string
	assetsFSCursor, distFSCursor unixfs.FSCursor
	assetsMux, distMux           srpc.Mux
}

// newPluginHostServerFsTracker constructs a new plugin host server fs tracker.
func (s *PluginHostServer) newPluginHostServerFsTracker(pluginID string) (keyed.Routine, *pluginHostServerFsTracker) {
	tr := &pluginHostServerFsTracker{
		s:                s,
		pluginID:         pluginID,
		resultPromiseCtr: promise.NewPromiseContainer[*pluginHostServerFsTrackerResult](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *pluginHostServerFsTracker) execute(rctx context.Context) error {
	resolve := func() error {
		t.resultPromiseCtr.SetPromise(nil)

		ctx, ctxCancel := context.WithCancel(rctx)
		defer ctxCancel()

		pluginID := t.pluginID
		if pluginID != t.s.pluginID {
			// if the plugin id is not the same as the plugin host (cross-plugin reference) add a plugin load directive
			_, _, pluginRef, err := bldr_plugin.ExLoadPlugin(ctx, t.s.b, false, pluginID, ctxCancel)
			if err != nil {
				return err
			}
			defer pluginRef.Release()
		}

		// build the plugin unixfs ids
		assetsUnixFSID := bldr_plugin.PluginAssetsFsId(pluginID)
		distUnixFSID := bldr_plugin.PluginDistFsId(pluginID)

		// build the access funcs
		assetsAccessFunc := unixfs_access.NewAccessUnixFSViaBusFunc(t.s.b, assetsUnixFSID, false)
		distAccessFunc := unixfs_access.NewAccessUnixFSViaBusFunc(t.s.b, distUnixFSID, false)

		// build the fscursors
		assetsFsCursor := unixfs_access.NewFSCursor(assetsAccessFunc)
		defer assetsFsCursor.Release()

		distFsCursor := unixfs_access.NewFSCursor(distAccessFunc)
		defer distFsCursor.Release()

		// build the muxes
		assetsMux, distMux := srpc.NewMux(nil), srpc.NewMux(nil)

		// build the servers
		assetsFsCursorServiceServer := unixfs_rpc_server.NewFSCursorService(assetsFsCursor)
		defer assetsFsCursorServiceServer.Release(true)

		distFsCursorServiceServer := unixfs_rpc_server.NewFSCursorService(distFsCursor)
		defer distFsCursorServiceServer.Release(true)

		// register to muxes
		_ = unixfs_rpc.SRPCRegisterFSCursorService(assetsMux, assetsFsCursorServiceServer)
		_ = unixfs_rpc.SRPCRegisterFSCursorService(distMux, distFsCursorServiceServer)

		// write result
		t.resultPromiseCtr.SetResult(&pluginHostServerFsTrackerResult{
			assetsUnixFSID: assetsUnixFSID,
			distUnixFSID:   distUnixFSID,
			assetsFSCursor: assetsFsCursor,
			distFSCursor:   distFsCursor,
			assetsMux:      assetsMux,
			distMux:        distMux,
		}, nil)

		// await context cancel
		<-ctx.Done()

		return context.Canceled
	}

	// retry loop (if plugins reload)
	for {
		rerr := resolve()

		// if outer context canceled, return error. otherwise retry.
		if err := rctx.Err(); err != nil {
			return context.Canceled
		}
		if rerr != nil && rerr != context.Canceled {
			return rerr
		}
	}
}
