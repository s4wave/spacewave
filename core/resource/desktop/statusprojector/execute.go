package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
	"github.com/sirupsen/logrus"
)

// Execute publishes Spacewave status into the host desktop tray tree.
func (c *Controller) Execute(ctx context.Context) error {
	le := c.GetLogger()
	le.Info("desktop tray status projector starting")

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
	le.Debug("desktop tray status projector found session controller")

	publisher, err := newHostDesktopTrayPublisher(ctx, c.GetBus())
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.Wrap(err, "open host desktop tray")
	}
	defer func() {
		if err := publisher.Release(context.Background()); err != nil {
			le.WithError(err).Warn("release desktop tray status publisher")
		}
	}()
	le.Debug("desktop tray status projector opened host desktop tray")

	launcher := newLauncherInfoWatcher(ctx, c.GetBus())
	return projectRuntimeTrayStatus(
		ctx,
		c.GetBus(),
		resource_listener.GetProcessStatusBroker(),
		sessionCtrl,
		launcher,
		publisher,
		le,
	)
}

func projectRuntimeTrayStatus(
	ctx context.Context,
	b bus.Bus,
	broker *resource_listener.StatusBroker,
	sessionCtrl session.SessionController,
	launcher *launcherInfoWatcher,
	publisher *desktopTrayPublisher,
	le *logrus.Entry,
) error {
	var prev *desktop_runtime.DesktopRuntimeState
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
		var changed bool
		prev, changed, err = publishDesktopTrayState(ctx, publisher, prev, current)
		if err != nil {
			releaseAll(releases)
			if ctx.Err() != nil {
				return nil
			}
			return errors.Wrap(err, "publish desktop tray status")
		}
		logDesktopTrayProjection(le, current, changed)

		ctxDone := waitAnyStatusChange(ctx, waitChs)
		releaseAll(releases)
		if ctxDone {
			return nil
		}
	}
}

func publishDesktopTrayState(
	ctx context.Context,
	publisher *desktopTrayPublisher,
	prev *desktop_runtime.DesktopRuntimeState,
	current *desktop_runtime.DesktopRuntimeState,
) (*desktop_runtime.DesktopRuntimeState, bool, error) {
	if prev != nil && prev.EqualVT(current) {
		return prev, false, nil
	}
	changed, err := publisher.Publish(ctx, current)
	if err != nil {
		return prev, false, err
	}
	return current.CloneVT(), changed, nil
}

func logDesktopTrayProjection(
	le *logrus.Entry,
	state *desktop_runtime.DesktopRuntimeState,
	changed bool,
) {
	if le == nil || state == nil {
		return
	}
	entry := le.WithFields(logrus.Fields{
		"changed":         changed,
		"status-text":     state.GetStatusText(),
		"sessions":        len(state.GetSessions()),
		"spaces":          len(state.GetSpaces()),
		"activity":        len(state.GetActivity()),
		"attention-items": len(state.GetAttentionItems()),
	})
	if changed {
		entry.Info("published desktop tray projection")
		return
	}
	entry.Debug("desktop tray projection unchanged")
}
