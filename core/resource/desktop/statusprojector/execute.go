package statusprojector

import (
	"context"

	"github.com/pkg/errors"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
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

	service := desktop_runtime.NewSRPCDesktopRuntimeResourceServiceClient(rootClient)
	return projectListenerStatus(ctx, resource_listener.GetProcessStatusBroker(), service)
}

func projectListenerStatus(
	ctx context.Context,
	broker *resource_listener.StatusBroker,
	service desktopRuntimePublisher,
) error {
	var prev *desktop_runtime.DesktopRuntimeState
	for {
		snapshot, waitCh := broker.Snapshot()
		current := BuildDesktopRuntimeStateFromListener(snapshot)
		var err error
		prev, _, err = publishDesktopRuntimeState(ctx, service, prev, current)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errors.Wrap(err, "publish desktop runtime status")
		}

		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
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
