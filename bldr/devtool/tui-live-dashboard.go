//go:build !js

package devtool

import (
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// BldrDevtoolTUIDashboard renders live devtool status snapshots.
type BldrDevtoolTUIDashboard struct {
	status   *devtool_status.BldrDevtoolStatus
	statusCh <-chan *devtool_status.BldrDevtoolStatus
}

// NewBldrDevtoolTUIDashboard creates a live dashboard component.
func NewBldrDevtoolTUIDashboard(
	initial *devtool_status.BldrDevtoolStatus,
	statusCh <-chan *devtool_status.BldrDevtoolStatus,
) *BldrDevtoolTUIDashboard {
	return &BldrDevtoolTUIDashboard{
		status:   normalizeDevtoolTUIStatus(initial),
		statusCh: statusCh,
	}
}

// GetStatusChannel returns the live status stream.
func (d *BldrDevtoolTUIDashboard) GetStatusChannel() <-chan *devtool_status.BldrDevtoolStatus {
	return d.statusCh
}

// Render returns the current dashboard text.
func (d *BldrDevtoolTUIDashboard) Render() string {
	return BuildDevtoolTUIDashboard(d.status)
}

// SetStatus updates the dashboard snapshot.
func (d *BldrDevtoolTUIDashboard) SetStatus(snapshot *devtool_status.BldrDevtoolStatus) {
	d.status = normalizeDevtoolTUIStatus(snapshot)
}

func normalizeDevtoolTUIStatus(
	snapshot *devtool_status.BldrDevtoolStatus,
) *devtool_status.BldrDevtoolStatus {
	if snapshot == nil {
		return devtool_status.EmptyBldrDevtoolStatus()
	}
	return snapshot
}
