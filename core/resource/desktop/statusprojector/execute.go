package statusprojector

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	desktop_tray "github.com/s4wave/spacewave/bldr/desktop/tray"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
)

type desktopRuntimePublisher interface {
	SetDesktopState(ctx context.Context, in *desktop_runtime.SetDesktopStateRequest) (*desktop_runtime.SetDesktopStateResponse, error)
}

const desktopRuntimeTeardownTimeout = 2 * time.Second

// Execute publishes listener status changes into the desktop runtime resource tree.
func (c *Controller) Execute(ctx context.Context) error {
	webRuntimeID := c.GetConfig().ResolvedWebRuntimeID()
	if webRuntimeID == "" {
		c.GetLogger().Debug("desktop runtime status projector disabled")
		return nil
	}

	webRuntime, _, webRuntimeRef, err := web_runtime.ExLookupWebRuntime(ctx, c.GetBus(), false, webRuntimeID)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.Wrap(err, "lookup desktop web runtime")
	}
	defer webRuntimeRef.Release()

	resourceClient, err := webRuntime.ConnectDesktopRuntimeResourceClient(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.Wrap(err, "connect desktop runtime resource")
	}
	defer resourceClient.Release()

	rootRef := resourceClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "access desktop runtime root resource")
	}

	hostRoot, _, hostRootRef, err := plugin_host_root.ExLookupRoot(ctx, c.GetBus(), false, nil, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.Wrap(err, "lookup plugin host root")
	}
	defer hostRootRef.Release()

	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, c.GetBus(), "", false, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.Wrap(err, "lookup session controller")
	}
	if sessionCtrlRef != nil {
		defer sessionCtrlRef.Release()
	}

	service := desktop_runtime.NewSRPCDesktopRuntimeResourceServiceClient(rootClient)
	traySource := desktop_tray.NewSRPCDesktopTrayResourceServiceClient(
		srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(hostRoot.GetMux()))),
	)
	trayTarget := desktop_tray.NewSRPCDesktopTrayResourceServiceClient(rootClient)
	launcher := newLauncherInfoWatcher(ctx, c.GetBus())
	return projectRuntimeStatusAndTray(
		ctx,
		c.GetBus(),
		resource_listener.GetProcessStatusBroker(),
		sessionCtrl,
		launcher,
		service,
		traySource,
		trayTarget,
		resourceClient,
	)
}

func projectRuntimeStatusAndTray(
	ctx context.Context,
	b bus.Bus,
	broker *resource_listener.StatusBroker,
	sessionCtrl session.SessionController,
	launcher *launcherInfoWatcher,
	service desktopRuntimePublisher,
	traySource desktop_tray.SRPCDesktopTrayResourceServiceClient,
	trayTarget desktop_tray.SRPCDesktopTrayResourceServiceClient,
	resourceClient *resource_client.Client,
) error {
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	errCh := make(chan error, 2)
	go func() {
		errCh <- projectRuntimeStatus(runCtx, b, broker, sessionCtrl, launcher, service)
	}()
	go func() {
		errCh <- desktop_tray.ReconcileDesktopTray(runCtx, traySource, trayTarget, resourceClient)
	}()

	err := <-errCh
	runCancel()
	secondErr := <-errCh
	if err == context.Canceled && ctx.Err() != nil {
		return nil
	}
	if err == context.Canceled && secondErr != nil && secondErr != context.Canceled {
		return secondErr
	}
	return err
}

func projectRuntimeStatus(
	ctx context.Context,
	b bus.Bus,
	broker *resource_listener.StatusBroker,
	sessionCtrl session.SessionController,
	launcher *launcherInfoWatcher,
	service desktopRuntimePublisher,
) (rerr error) {
	var prev *desktop_runtime.DesktopRuntimeState
	defer func() {
		publishDesktopRuntimeTeardownOnExit(ctx, service, prev, &rerr)
	}()
	for {
		snapshot, listenerWaitCh := broker.Snapshot()
		projection, sessionWaitChs, releases, err := snapshotSessionProjection(ctx, b, sessionCtrl)
		if err != nil {
			releaseAll(releases)
			if ctx.Err() != nil {
				return nil
			}
			return errors.Wrap(err, "snapshot session projection")
		}
		launcherInfo, launcherWaitCh := launcher.Snapshot()
		update, updateAttention := buildUpdateProjection(launcherInfo)
		projection.Update = update
		if updateAttention != nil {
			projection.AttentionItems = append(projection.AttentionItems, updateAttention)
		}

		waitChs := make([]<-chan struct{}, 0, len(sessionWaitChs)+2)
		waitChs = append(waitChs, listenerWaitCh)
		waitChs = append(waitChs, launcherWaitCh)
		waitChs = append(waitChs, sessionWaitChs...)

		current := BuildDesktopRuntimeState(snapshot, projection)
		prev, _, err = publishDesktopRuntimeState(ctx, service, prev, current)
		if err != nil {
			releaseAll(releases)
			if ctx.Err() != nil {
				return nil
			}
			return errors.Wrap(err, "publish desktop runtime status")
		}

		ctxDone := waitAnyStatusChange(ctx, waitChs)
		releaseAll(releases)
		if ctxDone {
			return nil
		}
	}
}

func publishDesktopRuntimeTeardownOnExit(
	ctx context.Context,
	service desktopRuntimePublisher,
	prev *desktop_runtime.DesktopRuntimeState,
	rerr *error,
) {
	if ctx.Err() != nil {
		return
	}
	_, err := publishDesktopRuntimeTeardownState(ctx, service, prev)
	if err != nil && rerr != nil && *rerr == nil && ctx.Err() == nil {
		*rerr = errors.Wrap(err, "publish desktop runtime teardown status")
	}
}

func publishDesktopRuntimeState(
	ctx context.Context,
	service desktopRuntimePublisher,
	prev *desktop_runtime.DesktopRuntimeState,
	current *desktop_runtime.DesktopRuntimeState,
) (*desktop_runtime.DesktopRuntimeState, bool, error) {
	if prev != nil && prev.EqualVT(current) {
		return prev, false, nil
	}
	_, err := service.SetDesktopState(ctx, &desktop_runtime.SetDesktopStateRequest{State: current})
	if err != nil {
		return prev, false, err
	}
	return current.CloneVT(), true, nil
}

func publishDesktopRuntimeTeardownState(
	ctx context.Context,
	service desktopRuntimePublisher,
	prev *desktop_runtime.DesktopRuntimeState,
) (*desktop_runtime.DesktopRuntimeState, error) {
	teardownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), desktopRuntimeTeardownTimeout)
	defer cancel()
	current := BuildDesktopRuntimeStateFromListener(resource_listener.ListenerStatus{})
	next, _, err := publishDesktopRuntimeState(teardownCtx, service, prev, current)
	return next, err
}
