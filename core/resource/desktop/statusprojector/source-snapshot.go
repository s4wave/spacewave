package statusprojector

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	desktop_runtime "github.com/s4wave/spacewave/bldr/web/electron/desktop-runtime"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector/updatepolicy"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
	"github.com/s4wave/spacewave/core/session"
)

type desktopTraySourceSnapshot struct {
	// listener is the current background listener state
	listener resource_listener.ListenerStatus
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
	launcher *launcherInfoWatcher,
) (*desktopTraySourceSnapshot, error) {
	listener, listenerWaitCh := broker.Snapshot()
	projection, sessionWaitChs, releases, err := snapshotSessionProjection(ctx, b, sessionCtrl)
	if err != nil {
		releaseAll(releases)
		return nil, err
	}
	launcherInfo, launcherWaitCh := launcher.Snapshot()
	update, updateAttention := updatepolicy.Build(launcherInfo)
	projection.Update = update
	if updateAttention != nil {
		projection.AttentionItems = append(projection.AttentionItems, updateAttention)
	}

	waitChs := make([]<-chan struct{}, 0, len(sessionWaitChs)+2)
	waitChs = append(waitChs, listenerWaitCh)
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
