package electron

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

type desktopTrayMirroredRuntime struct {
	web_runtime.WebRuntime

	controller *Controller
}

type pluginHostDesktopTray struct {
	resources *resource_client.Client
	trayRef   resource_client.ResourceRef
	tray      desktop_tray.SRPCDesktopTrayResourceServiceClient
}

func (r *Controller) hasTrayBackgroundPresence() bool {
	return r.electronInit.GetDesktopPresencePolicy() == DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND
}

func (r *Controller) reconcileDesktopTray(
	ctx context.Context,
	rt web_runtime.WebRuntime,
) error {
	r.le.Info("desktop tray reconciler starting")

	source, err := openPluginHostDesktopTray(ctx, r.bus)
	if err != nil {
		return err
	}
	defer source.Release()

	targetResources, err := rt.ConnectDesktopRuntimeResourceClient(ctx)
	if err != nil {
		return err
	}
	defer targetResources.Release()

	rootRef := targetResources.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		return err
	}

	trayTarget := desktop_tray.NewSRPCDesktopTrayResourceServiceClient(rootClient)
	err = desktop_tray.ReconcileDesktopTray(ctx, source.tray, trayTarget, targetResources)
	if err != nil && ctx.Err() != nil {
		return context.Canceled
	}
	return err
}

func (r *desktopTrayMirroredRuntime) Execute(ctx context.Context) error {
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	errCh := make(chan error, 2)
	go func() {
		errCh <- r.WebRuntime.Execute(runCtx)
	}()
	go func() {
		errCh <- r.controller.reconcileDesktopTray(runCtx, r.WebRuntime)
	}()

	err := <-errCh
	runCancel()
	secondErr := <-errCh
	if err == context.Canceled && ctx.Err() != nil {
		return context.Canceled
	}
	if err == nil && secondErr != nil && secondErr != context.Canceled {
		return secondErr
	}
	if err == context.Canceled && secondErr != nil && secondErr != context.Canceled {
		return secondErr
	}
	return err
}

func openPluginHostDesktopTray(ctx context.Context, b bus.Bus) (*pluginHostDesktopTray, error) {
	rpcClient := bifrost_rpc.NewBusClient(b)
	resourceService := bldr_resource.NewSRPCResourceServiceClientWithServiceID(
		rpcClient,
		bldr_plugin.HostServiceIDPrefix+bldr_resource.SRPCResourceServiceServiceID,
	)
	resources, err := resource_client.NewClient(ctx, resourceService)
	if err != nil {
		return nil, err
	}

	rootRef := resources.AccessRootResource()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		resources.Release()
		return nil, err
	}

	hostService := sdk_plugin_host.NewSRPCPluginHostResourceServiceClient(rootClient)
	resp, err := hostService.AccessDesktopTray(ctx, &sdk_plugin_host.AccessDesktopTrayRequest{})
	rootRef.Release()
	if err != nil {
		resources.Release()
		return nil, err
	}

	trayRef := resources.CreateResourceReference(resp.GetResourceId())
	trayClient, err := trayRef.GetClient()
	if err != nil {
		trayRef.Release()
		resources.Release()
		return nil, err
	}
	return &pluginHostDesktopTray{
		resources: resources,
		trayRef:   trayRef,
		tray:      desktop_tray.NewSRPCDesktopTrayResourceServiceClient(trayClient),
	}, nil
}

func (r *pluginHostDesktopTray) Release() {
	r.trayRef.Release()
	r.resources.Release()
}

// _ is a type assertion
var _ web_runtime.WebRuntime = ((*desktopTrayMirroredRuntime)(nil))
