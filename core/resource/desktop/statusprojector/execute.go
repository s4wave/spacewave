package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/logpolicy"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
	"github.com/sirupsen/logrus"
)

// Execute publishes Spacewave status into the host desktop tray tree.
func (c *Controller) Execute(ctx context.Context) error {
	if c.statusBroker == nil {
		return errors.New("listener status broker is not injected")
	}

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
		c.statusBroker,
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
		snapshot, err := snapshotDesktopTraySources(ctx, b, broker, sessionCtrl, launcher)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errors.Wrap(err, "snapshot desktop tray sources")
		}

		current := snapshot.buildRuntimeState()
		var changed bool
		previous := prev
		prev, changed, err = publishDesktopTrayState(ctx, publisher, prev, current)
		if err != nil {
			snapshot.release()
			if ctx.Err() != nil {
				return nil
			}
			return errors.Wrap(err, "publish desktop tray status")
		}
		logDesktopTrayProjection(le, previous, current, changed)

		ctxDone := snapshot.wait(ctx)
		snapshot.release()
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
	prev *desktop_runtime.DesktopRuntimeState,
	current *desktop_runtime.DesktopRuntimeState,
	changed bool,
) {
	if le == nil || current == nil {
		return
	}
	entry := le.WithFields(logrus.Fields{
		"changed":         changed,
		"status-text":     current.GetStatusText(),
		"sessions":        len(current.GetSessions()),
		"spaces":          len(current.GetSpaces()),
		"activity":        len(current.GetActivity()),
		"attention-items": len(current.GetAttentionItems()),
	})
	decision := logpolicy.Classify(prev, current, changed)
	if decision.Level == logpolicy.LevelInfo {
		entry.Info(decision.Message)
		return
	}
	entry.Debug(decision.Message)
}
