//go:build !js

package devtool

import (
	tui "github.com/grindlemire/go-tui"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// BldrDevtoolTUIDashboard renders live devtool status snapshots.
type BldrDevtoolTUIDashboard struct {
	status   *tui.State[*devtool_status.BldrDevtoolStatus]
	statusCh <-chan *devtool_status.BldrDevtoolStatus
}

// NewBldrDevtoolTUIDashboard creates a live dashboard component.
func NewBldrDevtoolTUIDashboard(
	initial *devtool_status.BldrDevtoolStatus,
	statusCh <-chan *devtool_status.BldrDevtoolStatus,
) *BldrDevtoolTUIDashboard {
	return &BldrDevtoolTUIDashboard{
		status:   tui.NewState(normalizeDevtoolTUIStatus(initial)),
		statusCh: statusCh,
	}
}

// BindApp binds dashboard state to the go-tui app.
func (d *BldrDevtoolTUIDashboard) BindApp(app *tui.App) {
	d.status.BindApp(app)
}

// Render returns the current dashboard tree.
func (d *BldrDevtoolTUIDashboard) Render(app *tui.App) *tui.Element {
	return BuildDevtoolTUIDashboardElement(d.status.Get())
}

// KeyMap returns dashboard key bindings.
func (d *BldrDevtoolTUIDashboard) KeyMap() tui.KeyMap {
	return tui.KeyMap{
		tui.OnStop(tui.KeyEscape, func(ke tui.KeyEvent) { ke.App().Stop() }),
		tui.OnStop(tui.KeyCtrlC, func(ke tui.KeyEvent) { ke.App().Stop() }),
		tui.OnStop(tui.Rune('q'), func(ke tui.KeyEvent) { ke.App().Stop() }),
	}
}

// Watchers connects snapshot updates to go-tui's event loop.
func (d *BldrDevtoolTUIDashboard) Watchers() []tui.Watcher {
	if d.statusCh == nil {
		return nil
	}
	return []tui.Watcher{
		tui.Watch(d.statusCh, d.setStatus),
	}
}

func (d *BldrDevtoolTUIDashboard) setStatus(snapshot *devtool_status.BldrDevtoolStatus) {
	d.status.Set(normalizeDevtoolTUIStatus(snapshot))
}

func normalizeDevtoolTUIStatus(
	snapshot *devtool_status.BldrDevtoolStatus,
) *devtool_status.BldrDevtoolStatus {
	if snapshot == nil {
		return devtool_status.EmptyBldrDevtoolStatus()
	}
	return snapshot
}

// _ is a type assertion
var (
	_ tui.AppBinder       = ((*BldrDevtoolTUIDashboard)(nil))
	_ tui.Component       = ((*BldrDevtoolTUIDashboard)(nil))
	_ tui.KeyListener     = ((*BldrDevtoolTUIDashboard)(nil))
	_ tui.WatcherProvider = ((*BldrDevtoolTUIDashboard)(nil))
)
