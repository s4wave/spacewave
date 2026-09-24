package plugin_host_resource

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
)

// InitialCapabilityRegistrationDoneFunc reports whether the plugin completed
// its initial capability-registration pass before its instance ended.
type InitialCapabilityRegistrationDoneFunc func(complete bool)

// PluginHostRoot is the root resource a plugin reaches through its host. It
// implements PluginHostResourceService: filesystem access, the host volume
// proxy, state atoms, the desktop tray, plugin info, and the initial
// capability-registration signal.
type PluginHostRoot struct {
	// pluginID is the owning plugin's id.
	pluginID string
	// entrypoint is the plugin entrypoint name.
	entrypoint string
	// distFS is the handle to the plugin's dist filesystem.
	distFS *unixfs.FSHandle
	// assetsFS is the handle to the plugin's assets filesystem.
	assetsFS *unixfs.FSHandle
	// proxyHostVol proxies the host volume into the plugin.
	proxyHostVol *volume_rpc_server.ProxyVolume
	// stateAtomMgr manages state atoms for the plugin.
	stateAtomMgr *resource_state.StateAtomManager
	// hostRoot is the host-side root resource.
	hostRoot *plugin_host_root.Root
	// mux is the SRPC invoker for the plugin.
	mux srpc.Invoker
	// registrationDoneOnce guards initial capability registration.
	registrationDoneOnce sync.Once
	// registrationDone signals initial capability registration completed.
	registrationDone InitialCapabilityRegistrationDoneFunc
}

// NewPluginHostRoot constructs a new PluginHostRoot.
func NewPluginHostRoot(
	b bus.Bus,
	pluginID, entrypoint string,
	distFS, assetsFS *unixfs.FSHandle,
	proxyHostVol *volume_rpc_server.ProxyVolume,
	hostRoot *plugin_host_root.Root,
	stateAtomObjectStoreID, stateAtomVolumeID string,
	registrationDone InitialCapabilityRegistrationDoneFunc,
) *PluginHostRoot {
	r := &PluginHostRoot{
		pluginID:         pluginID,
		entrypoint:       entrypoint,
		distFS:           distFS,
		assetsFS:         assetsFS,
		proxyHostVol:     proxyHostVol,
		hostRoot:         hostRoot,
		registrationDone: registrationDone,
	}
	r.stateAtomMgr = resource_state.NewStateAtomManager(b, stateAtomObjectStoreID, stateAtomVolumeID)
	mux := resource_server.NewResourceMux(func(m srpc.Mux) error {
		return sdk_plugin_host.SRPCRegisterPluginHostResourceService(m, r)
	})
	r.mux = mux
	return r
}

// GetMux returns the RPC mux for the root resource.
func (r *PluginHostRoot) GetMux() srpc.Invoker {
	return r.mux
}

// Release releases all resources held by the root.
func (r *PluginHostRoot) Release() {
	r.finishInitialCapabilityRegistration(false)
	r.stateAtomMgr.Release()
}

// CompleteInitialCapabilityRegistration marks the plugin's initial capability
// registration pass complete.
func (r *PluginHostRoot) CompleteInitialCapabilityRegistration(
	context.Context,
	*sdk_plugin_host.CompleteInitialCapabilityRegistrationRequest,
) (*sdk_plugin_host.CompleteInitialCapabilityRegistrationResponse, error) {
	r.finishInitialCapabilityRegistration(true)
	return &sdk_plugin_host.CompleteInitialCapabilityRegistrationResponse{}, nil
}

// finishInitialCapabilityRegistration signals initial capability
// registration completed (or was interrupted) exactly once.
func (r *PluginHostRoot) finishInitialCapabilityRegistration(complete bool) {
	r.registrationDoneOnce.Do(func() {
		if r.registrationDone != nil {
			r.registrationDone(complete)
		}
	})
}

// serveChildResource constructs a child resource whose invoker is a new
// SRPC mux with the handler registered by register.
func (r *PluginHostRoot) serveChildResource(
	ctx context.Context,
	register func(mux srpc.Mux) error,
) (uint32, error) {
	_, id, err := resource_server.ConstructChildResource(ctx, func(_ context.Context) (srpc.Invoker, struct{}, func(), error) {
		mux := srpc.NewMux()
		if err := register(mux); err != nil {
			return nil, struct{}{}, nil, err
		}
		return mux, struct{}{}, nil, nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// AccessAssetsFS returns a resource ID for the plugin's assets filesystem.
func (r *PluginHostRoot) AccessAssetsFS(
	ctx context.Context,
	req *sdk_plugin_host.AccessAssetsFSRequest,
) (*sdk_plugin_host.AccessAssetsFSResponse, error) {
	id, err := r.serveChildResource(ctx, func(mux srpc.Mux) error {
		return mux.Register(unixfs_rpc.NewSRPCFSCursorServiceHandler(
			unixfs_rpc_server.NewFSCursorServiceWithHandle(r.assetsFS),
			"",
		))
	})
	if err != nil {
		return nil, err
	}
	return &sdk_plugin_host.AccessAssetsFSResponse{ResourceId: id}, nil
}

// AccessDistFS returns a resource ID for the plugin's dist filesystem.
func (r *PluginHostRoot) AccessDistFS(
	ctx context.Context,
	req *sdk_plugin_host.AccessDistFSRequest,
) (*sdk_plugin_host.AccessDistFSResponse, error) {
	id, err := r.serveChildResource(ctx, func(mux srpc.Mux) error {
		return mux.Register(unixfs_rpc.NewSRPCFSCursorServiceHandler(
			unixfs_rpc_server.NewFSCursorServiceWithHandle(r.distFS),
			"",
		))
	})
	if err != nil {
		return nil, err
	}
	return &sdk_plugin_host.AccessDistFSResponse{ResourceId: id}, nil
}

// AccessVolume returns a resource ID for the plugin's host volume.
func (r *PluginHostRoot) AccessVolume(
	ctx context.Context,
	req *sdk_plugin_host.AccessVolumeRequest,
) (*sdk_plugin_host.AccessVolumeResponse, error) {
	id, err := r.serveChildResource(ctx, func(mux srpc.Mux) error {
		return volume_rpc_server.RegisterProxyVolumeWithPrefix(mux, r.proxyHostVol, "")
	})
	if err != nil {
		return nil, err
	}
	return &sdk_plugin_host.AccessVolumeResponse{ResourceId: id}, nil
}

// AccessStateAtom returns a resource ID for a state atom store.
func (r *PluginHostRoot) AccessStateAtom(
	ctx context.Context,
	req *sdk_plugin_host.AccessStateAtomRequest,
) (*sdk_plugin_host.AccessStateAtomResponse, error) {
	storeID := req.GetStoreId()
	if storeID == "" {
		storeID = resource_state.DefaultStateAtomStoreID
	}
	_, id, err := resource_server.ConstructChildResource(ctx, func(subCtx context.Context) (srpc.Invoker, struct{}, func(), error) {
		store, err := r.stateAtomMgr.GetOrCreateStore(subCtx, storeID)
		if err != nil {
			return nil, struct{}{}, nil, err
		}
		res := resource_state.NewStateAtomResource(store)
		return res.GetMux(), struct{}{}, nil, nil
	})
	if err != nil {
		return nil, err
	}
	return &sdk_plugin_host.AccessStateAtomResponse{ResourceId: id}, nil
}

// AccessDesktopTray returns a resource ID for the process-lifetime desktop tray.
func (r *PluginHostRoot) AccessDesktopTray(
	ctx context.Context,
	req *sdk_plugin_host.AccessDesktopTrayRequest,
) (*sdk_plugin_host.AccessDesktopTrayResponse, error) {
	_, id, err := resource_server.ConstructChildResource(ctx, func(_ context.Context) (srpc.Invoker, struct{}, func(), error) {
		return r.hostRoot.GetDesktopTray().GetMux(), struct{}{}, nil, nil
	})
	if err != nil {
		return nil, err
	}
	return &sdk_plugin_host.AccessDesktopTrayResponse{ResourceId: id}, nil
}

// GetPluginInfo returns information about the running plugin.
func (r *PluginHostRoot) GetPluginInfo(
	ctx context.Context,
	req *sdk_plugin_host.GetPluginInfoRequest,
) (*sdk_plugin_host.GetPluginInfoResponse, error) {
	return &sdk_plugin_host.GetPluginInfoResponse{
		PluginId:   r.pluginID,
		Entrypoint: r.entrypoint,
	}, nil
}

// _ is a type assertion
var _ sdk_plugin_host.SRPCPluginHostResourceServiceServer = (*PluginHostRoot)(nil)
