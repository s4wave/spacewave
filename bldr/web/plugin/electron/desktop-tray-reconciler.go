package electron

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/routine"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

var (
	errDesktopTrayReconcilerEnded = errors.New("desktop tray reconciler ended")
	desktopTrayReconcilerBackoff  = &backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			InitialInterval:     100,
			MaxInterval:         1200,
			RandomizationFactor: 0.1,
		},
	}
)

type desktopTrayMirroredRuntime struct {
	web_runtime.WebRuntime

	controller              *Controller
	reconcileDesktopTrayRaw func(context.Context, web_runtime.WebRuntime) error
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

func (r *Controller) reconcileDesktopTrayUntilCanceled(
	ctx context.Context,
	rt web_runtime.WebRuntime,
) error {
	return runDesktopTrayReconcilerUntilCanceled(ctx, r.le, func(ctx context.Context) error {
		return r.reconcileDesktopTray(ctx, rt)
	})
}

func runDesktopTrayReconcilerUntilCanceled(
	ctx context.Context,
	le *logrus.Entry,
	reconcile func(context.Context) error,
) error {
	opts := []routine.Option{routine.WithRetry(desktopTrayReconcilerBackoff)}
	if le != nil {
		opts = append(opts, routine.WithExitLogger(le.WithField("routine", "desktop-tray-reconciler")))
	}

	rc := routine.NewRoutineContainer(opts...)
	rc.SetRoutine(func(ctx context.Context) error {
		err := reconcile(ctx)
		if ctx.Err() != nil {
			return context.Canceled
		}
		if err == nil {
			return errDesktopTrayReconcilerEnded
		}
		return err
	})
	rc.SetContext(ctx, false)

	<-ctx.Done()
	waitCh, _ := rc.SetRoutine(nil)
	if waitCh != nil {
		<-waitCh
	}
	rc.ClearContext()
	return context.Canceled
}

func (r *desktopTrayMirroredRuntime) Execute(ctx context.Context) error {
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	runtimeErrCh := make(chan error, 1)
	go func() {
		runtimeErrCh <- r.WebRuntime.Execute(runCtx)
	}()

	trayErrCh := make(chan error, 1)
	go func() {
		trayErrCh <- r.executeDesktopTrayReconciler(runCtx)
	}()

	select {
	case err := <-runtimeErrCh:
		runCancel()
		trayErr := <-trayErrCh
		if err == context.Canceled && ctx.Err() != nil {
			return context.Canceled
		}
		if err == nil && trayErr != nil && trayErr != context.Canceled {
			return trayErr
		}
		if err == context.Canceled && trayErr != nil && trayErr != context.Canceled {
			return trayErr
		}
		return err
	case err := <-trayErrCh:
		runCancel()
		runtimeErr := <-runtimeErrCh
		if err == context.Canceled && ctx.Err() != nil {
			return context.Canceled
		}
		if err != nil && err != context.Canceled {
			return err
		}
		return runtimeErr
	}
}

func (r *desktopTrayMirroredRuntime) executeDesktopTrayReconciler(ctx context.Context) error {
	if r.reconcileDesktopTrayRaw != nil {
		return runDesktopTrayReconcilerUntilCanceled(ctx, r.controller.le, func(ctx context.Context) error {
			return r.reconcileDesktopTrayRaw(ctx, r.WebRuntime)
		})
	}
	return r.controller.reconcileDesktopTrayUntilCanceled(ctx, r.WebRuntime)
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
