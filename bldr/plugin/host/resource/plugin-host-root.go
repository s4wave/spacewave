package plugin_host_resource

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	"github.com/sirupsen/logrus"
)

// PluginHostRoot is the root resource handler for plugins.
// It wraps all plugin resources and implements PluginHostResourceService.
type PluginHostRoot struct {
	le           *logrus.Entry
	b            bus.Bus
	pluginID     string
	entrypoint   string
	distFS       *unixfs.FSHandle
	assetsFS     *unixfs.FSHandle
	proxyHostVol *volume_rpc_server.ProxyVolume
	stateAtomMgr *resource_state.StateAtomManager
	mux          srpc.Invoker
}

// NewPluginHostRoot constructs a new PluginHostRoot.
func NewPluginHostRoot(
	le *logrus.Entry,
	b bus.Bus,
	pluginID, entrypoint string,
	distFS, assetsFS *unixfs.FSHandle,
	proxyHostVol *volume_rpc_server.ProxyVolume,
	stateAtomObjectStoreID, stateAtomVolumeID string,
) *PluginHostRoot {
	r := &PluginHostRoot{
		le:           le,
		b:            b,
		pluginID:     pluginID,
		entrypoint:   entrypoint,
		distFS:       distFS,
		assetsFS:     assetsFS,
		proxyHostVol: proxyHostVol,
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
	r.stateAtomMgr.Release()
}

// AccessAssetsFS returns a resource ID for the plugin's assets filesystem.
func (r *PluginHostRoot) AccessAssetsFS(
	ctx context.Context,
	req *sdk_plugin_host.AccessAssetsFSRequest,
) (*sdk_plugin_host.AccessAssetsFSResponse, error) {
	_, id, err := resource_server.ConstructChildResource(ctx, func(_ context.Context) (srpc.Invoker, struct{}, func(), error) {
		mux := srpc.NewMux()
		err := mux.Register(unixfs_rpc.NewSRPCFSCursorServiceHandler(
			unixfs_rpc_server.NewFSCursorServiceWithHandle(r.assetsFS),
			"",
		))
		if err != nil {
			return nil, struct{}{}, nil, err
		}
		return mux, struct{}{}, nil, nil
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
	_, id, err := resource_server.ConstructChildResource(ctx, func(_ context.Context) (srpc.Invoker, struct{}, func(), error) {
		mux := srpc.NewMux()
		err := mux.Register(unixfs_rpc.NewSRPCFSCursorServiceHandler(
			unixfs_rpc_server.NewFSCursorServiceWithHandle(r.distFS),
			"",
		))
		if err != nil {
			return nil, struct{}{}, nil, err
		}
		return mux, struct{}{}, nil, nil
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
	_, id, err := resource_server.ConstructChildResource(ctx, func(_ context.Context) (srpc.Invoker, struct{}, func(), error) {
		mux := srpc.NewMux()
		err := volume_rpc_server.RegisterProxyVolumeWithPrefix(mux, r.proxyHostVol, "")
		if err != nil {
			return nil, struct{}{}, nil, err
		}
		return mux, struct{}{}, nil, nil
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
