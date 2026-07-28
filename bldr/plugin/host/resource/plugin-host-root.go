package plugin_host_resource

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_server "github.com/s4wave/spacewave/db/unixfs/rpc/server"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_objecttype_registry "github.com/s4wave/spacewave/sdk/objecttype/registry"
	"github.com/sirupsen/logrus"
)

// InitialCapabilityRegistrationDoneFunc reports whether the plugin completed
// its initial capability-registration pass before its instance ended.
type InitialCapabilityRegistrationDoneFunc func(complete bool)

// PluginHostRoot is the root resource handler for plugins.
// It wraps all plugin resources and implements PluginHostResourceService.
type PluginHostRoot struct {
	ctx                  context.Context
	le                   *logrus.Entry
	b                    bus.Bus
	pluginID             string
	entrypoint           string
	distFS               *unixfs.FSHandle
	assetsFS             *unixfs.FSHandle
	proxyHostVol         *volume_rpc_server.ProxyVolume
	stateAtomMgr         *resource_state.StateAtomManager
	hostRoot             *plugin_host_root.Root
	mux                  srpc.Invoker
	releaseOnce          sync.Once
	registrationDoneOnce sync.Once
	registrationDone     InitialCapabilityRegistrationDoneFunc
	objectTypeMtx        sync.Mutex
	objectTypes          map[*objectTypeRegistration]struct{}
	released             bool
}

type objectTypeRegistration struct {
	once       sync.Once
	resources  *resource_client.Client
	serviceRef directive.Reference
	ref        resource_client.ResourceRef
}

func (r *objectTypeRegistration) release() {
	r.once.Do(func() {
		if r.ref != nil {
			r.ref.Release()
		}
		r.resources.Release()
		r.serviceRef.Release()
	})
}

// NewPluginHostRoot constructs a new PluginHostRoot.
func NewPluginHostRoot(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	pluginID, entrypoint string,
	distFS, assetsFS *unixfs.FSHandle,
	proxyHostVol *volume_rpc_server.ProxyVolume,
	hostRoot *plugin_host_root.Root,
	stateAtomObjectStoreID, stateAtomVolumeID string,
	registrationDone InitialCapabilityRegistrationDoneFunc,
) *PluginHostRoot {
	r := &PluginHostRoot{
		ctx:              ctx,
		le:               le,
		b:                b,
		pluginID:         pluginID,
		entrypoint:       entrypoint,
		distFS:           distFS,
		assetsFS:         assetsFS,
		proxyHostVol:     proxyHostVol,
		hostRoot:         hostRoot,
		objectTypes:      make(map[*objectTypeRegistration]struct{}),
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
	r.releaseOnce.Do(func() {
		r.objectTypeMtx.Lock()
		r.released = true
		registrations := make([]*objectTypeRegistration, 0, len(r.objectTypes))
		for registration := range r.objectTypes {
			registrations = append(registrations, registration)
		}
		clear(r.objectTypes)
		r.objectTypeMtx.Unlock()

		for _, registration := range registrations {
			registration.release()
		}
		r.stateAtomMgr.Release()
	})
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

func (r *PluginHostRoot) finishInitialCapabilityRegistration(complete bool) {
	r.registrationDoneOnce.Do(func() {
		if r.registrationDone != nil {
			r.registrationDone(complete)
		}
	})
}

// RegisterObjectType registers an ObjectType served by the running plugin.
func (r *PluginHostRoot) RegisterObjectType(
	ctx context.Context,
	req *sdk_plugin_host.RegisterObjectTypeRequest,
) (*sdk_plugin_host.RegisterObjectTypeResponse, error) {
	pluginClient, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	invokers, _, serviceRef, err := bifrost_rpc.ExLookupRpcService(
		r.ctx,
		r.b,
		resource.SRPCResourceServiceServiceID,
		"",
		true,
		nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "lookup core Resource service")
	}
	if len(invokers) == 0 {
		serviceRef.Release()
		return nil, errors.New("core Resource service not found")
	}
	resourceService := resource.NewSRPCResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invokers[0]))),
	)
	resources, err := resource_client.NewClient(r.ctx, resourceService)
	if err != nil {
		serviceRef.Release()
		return nil, errors.Wrap(err, "connect to core ObjectType registry")
	}
	registration := &objectTypeRegistration{
		resources:  resources,
		serviceRef: serviceRef,
	}
	rootRef := resources.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		registration.release()
		return nil, err
	}
	service := s4wave_objecttype_registry.NewSRPCObjectTypeRegistryResourceServiceClient(rootClient)
	resp, err := service.RegisterObjectType(ctx, &s4wave_objecttype_registry.RegisterObjectTypeRequest{
		TypeId:   req.GetTypeId(),
		PluginId: r.pluginID,
		Metadata: req.GetMetadata(),
	})
	rootRef.Release()
	if err != nil {
		registration.release()
		return nil, err
	}
	if resp.GetResourceId() == 0 {
		registration.release()
		return nil, errors.New("core ObjectType registration returned zero resource id")
	}
	registration.ref = resources.CreateResourceReference(resp.GetResourceId())
	r.objectTypeMtx.Lock()
	if r.released {
		r.objectTypeMtx.Unlock()
		registration.release()
		return nil, errors.New("plugin host root is released")
	}
	r.objectTypes[registration] = struct{}{}
	r.objectTypeMtx.Unlock()

	resourceID, err := pluginClient.AddResource(srpc.NewMux(), func() {
		r.releaseObjectTypeRegistration(registration)
	})
	if err != nil {
		r.releaseObjectTypeRegistration(registration)
		return nil, err
	}
	return &sdk_plugin_host.RegisterObjectTypeResponse{ResourceId: resourceID}, nil
}

func (r *PluginHostRoot) releaseObjectTypeRegistration(registration *objectTypeRegistration) {
	r.objectTypeMtx.Lock()
	_, ok := r.objectTypes[registration]
	if ok {
		delete(r.objectTypes, registration)
	}
	r.objectTypeMtx.Unlock()
	if ok {
		registration.release()
	}
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
