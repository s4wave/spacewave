package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
)

type desktopRuntimePublisher interface {
	SetDesktopState(ctx context.Context, in *desktop_runtime.SetDesktopStateRequest) (*desktop_runtime.SetDesktopStateResponse, error)
}

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
	return projectRuntimeStatus(ctx, c.GetBus(), resource_listener.GetProcessStatusBroker(), sessionCtrl, service)
}

func projectRuntimeStatus(
	ctx context.Context,
	b bus.Bus,
	broker *resource_listener.StatusBroker,
	sessionCtrl session.SessionController,
	service desktopRuntimePublisher,
) error {
	var prev *desktop_runtime.DesktopRuntimeState
	for {
		snapshot, listenerWaitCh := broker.Snapshot()
		projection, sessionWaitChs, releases, err := snapshotSessionProjection(ctx, b, sessionCtrl)
		if err != nil {
			releaseAll(releases)
			return errors.Wrap(err, "snapshot session projection")
		}
		waitChs := make([]<-chan struct{}, 0, len(sessionWaitChs)+1)
		waitChs = append(waitChs, listenerWaitCh)
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
