package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/updatepolicy"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
)

type desktopTraySourceSnapshot struct {
	// listener is the current background listener state, or nil when this
	// process owns no listener
	listener *resource_listener.ListenerStatus
	// projection is the current Spacewave semantic tray projection
	projection *SessionProjection
	// waitChs wake the next projector loop after a source changes
	waitChs []<-chan struct{}
	// releases closes resources held by this snapshot after publication waits
	releases []func()
}

func snapshotDesktopTraySources(
	ctx context.Context,
	b bus.Bus,
	broker *resource_listener.StatusBroker,
	sessionCtrl session.SessionController,
	launcher *spacewave_launcher.InfoWatcher,
) (*desktopTraySourceSnapshot, error) {
	// Read the listener status when this process owns a listener.
	var listener *resource_listener.ListenerStatus
	var listenerWaitCh <-chan struct{}
	if broker != nil {
		var status resource_listener.ListenerStatus
		status, listenerWaitCh = broker.Snapshot()
		listener = &status
	}

	// Read the session projection.
	projection, sessionWaitChs, releases, err := snapshotSessionProjection(ctx, b, sessionCtrl)
	if err != nil {
		releaseAll(releases)
		return nil, err
	}

	// Fold the launcher update state into the projection.
	launcherInfo, launcherWaitCh := launcher.Snapshot()
	update, updateAttention := updatepolicy.Build(launcherInfo)
	projection.Update = update
	if updateAttention != nil {
		projection.AttentionItems = append(projection.AttentionItems, updateAttention)
	}

	// Wake the next projection when any source changes.
	waitChs := make([]<-chan struct{}, 0, len(sessionWaitChs)+2)
	if listenerWaitCh != nil {
		waitChs = append(waitChs, listenerWaitCh)
	}
	waitChs = append(waitChs, launcherWaitCh)
	waitChs = append(waitChs, sessionWaitChs...)
	return &desktopTraySourceSnapshot{
		listener:   listener,
		projection: projection,
		waitChs:    waitChs,
		releases:   releases,
	}, nil
}

func (s *desktopTraySourceSnapshot) buildRuntimeState() *desktop_runtime.DesktopRuntimeState {
	return BuildDesktopRuntimeState(s.listener, s.projection)
}

func (s *desktopTraySourceSnapshot) wait(ctx context.Context) bool {
	return waitAnyStatusChange(ctx, s.waitChs)
}

func (s *desktopTraySourceSnapshot) release() {
	releaseAll(s.releases)
}
