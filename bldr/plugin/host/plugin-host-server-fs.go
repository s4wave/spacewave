package plugin_host

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
)

// pluginHostServerFsTracker tracks a plugin fs for ongoing rpc calls for the plugin host server.
type pluginHostServerFsTracker struct {
	// s owns the bus and tracker lifetime.
	s *PluginHostServer
	// pluginID selects the family and optional immutable manifest.
	pluginID string
	// resultPromiseCtr publishes available services or their load failure.
	resultPromiseCtr *promise.PromiseContainer[*pluginHostServerFsTrackerResult]
}

// pluginHostServerFsTrackerResult exposes filesystem services for one retained execution.
type pluginHostServerFsTrackerResult struct {
	// assetsMux serves immutable plugin assets.
	assetsMux srpc.Mux
	// distMux serves immutable plugin distribution files.
	distMux srpc.Mux
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

// execute retains the demanded plugin and filesystem services until release or load failure.
func (t *pluginHostServerFsTracker) execute(rctx context.Context) (rerr error) {
	// The keyed tracker owns retry; publish each terminal load failure to waiters.
	t.resultPromiseCtr.SetPromise(nil)
	defer func() {
		if rerr != nil && rctx.Err() == nil {
			t.resultPromiseCtr.SetResult(nil, rerr)
		}
	}()

	ctx, ctxCancel := context.WithCancelCause(rctx)
	defer ctxCancel(context.Canceled)

	pluginID := t.pluginID
	familyID, manifestRoot, err := bldr_plugin.ParsePluginArtifactID(pluginID, false)
	if err != nil {
		return err
	}
	self := familyID == t.s.pluginID && (manifestRoot == "" || manifestRoot == t.s.manifestSnapshot.GetManifestRef().GetRootRef().GetHash().MarshalString())
	if !self {
		// Retain execution without waiting for startup: a worker needs these files
		// before it can connect and report capability-registration completion.
		var running atomic.Int32
		load, pluginRef, err := t.s.b.AddDirective(
			bldr_plugin.NewLoadPluginAtManifest(familyID, t.s.instanceKey, manifestRoot),
			directive.NewCallbackHandler(
				func(directive.AttachedValue) { running.Add(1) },
				func(directive.AttachedValue) { running.Add(-1) },
				func() { ctxCancel(errors.New("plugin file provider was released")) },
			),
		)
		if err != nil {
			return err
		}
		defer pluginRef.Release()
		defer load.AddIdleCallback(func(idle bool, errs []error) {
			if !idle || running.Load() != 0 {
				return
			}
			for _, err := range errs {
				if err != nil && !errors.Is(err, context.Canceled) {
					ctxCancel(err)
					return
				}
			}
			ctxCancel(errors.New("plugin executable is unavailable: " + pluginID))
		})()
	}

	// Resolve filesystem providers without waiting for backend registration.
	assetsUnixFSID := bldr_plugin.PluginAssetsFsId(pluginID)
	distUnixFSID := bldr_plugin.PluginDistFsId(pluginID)

	assetsAccessFunc := t.accessFiles(ctx, assetsUnixFSID)
	distAccessFunc := t.accessFiles(ctx, distUnixFSID)

	// Retain cursors and their RPC services for this execution.
	assetsFsCursor := unixfs_access.NewFSCursor(assetsAccessFunc)
	defer assetsFsCursor.Release()

	distFsCursor := unixfs_access.NewFSCursor(distAccessFunc)
	defer distFsCursor.Release()

	assetsMux, distMux := srpc.NewMux(nil), srpc.NewMux(nil)

	assetsFsCursorServiceServer := unixfs_rpc_server.NewFSCursorService(assetsFsCursor)
	defer assetsFsCursorServiceServer.Release(true)

	distFsCursorServiceServer := unixfs_rpc_server.NewFSCursorService(distFsCursor)
	defer distFsCursorServiceServer.Release(true)

	// Publish services only after both cursors have cleanup registered.
	_ = unixfs_rpc.SRPCRegisterFSCursorService(assetsMux, assetsFsCursorServiceServer)
	_ = unixfs_rpc.SRPCRegisterFSCursorService(distMux, distFsCursorServiceServer)

	t.resultPromiseCtr.SetResult(&pluginHostServerFsTrackerResult{
		assetsMux: assetsMux,
		distMux:   distMux,
	}, nil)

	// A terminal load failure cancels active reads before retrying the tracker.
	<-ctx.Done()

	return context.Cause(ctx)
}

// accessFiles binds filesystem acquisition to the tracked execution without waiting
// for registration. Worker startup can fetch its modules before it reports ready;
// failed loading cancels a blocked read and invalidates acquired cursors.
func (t *pluginHostServerFsTracker) accessFiles(lifetime context.Context, id string) unixfs_access.AccessUnixFSFunc {
	access := unixfs_access.NewAccessUnixFSViaBusFunc(t.s.b, id, false)
	return func(caller context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		ctx, cancel := context.WithCancelCause(caller)
		stop := context.AfterFunc(lifetime, func() {
			cancel(context.Cause(lifetime))
			if released != nil {
				released()
			}
		})
		handle, release, err := access(ctx, released)
		if err != nil {
			stop()
			cause := context.Cause(ctx)
			cancel(context.Canceled)
			if cause != nil {
				err = cause
			}
			return nil, nil, err
		}

		return handle, func() {
			stop()
			cancel(context.Canceled)
			release()
		}, nil
	}
}
