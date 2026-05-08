package status

import (
	"context"
	"slices"
	"strings"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
)

// AttachPluginStatus adapts PluginHost scheduler status into Bldr Devtool Status.
func AttachPluginStatus(
	ctx context.Context,
	producer *BldrDevtoolStatusProducer,
	ctrl *plugin_host_scheduler.Controller,
) {
	adapter := &pluginStatusAdapter{producer: producer}
	statusCtr := ctrl.GetPluginStatusCtr()
	current := statusCtr.GetValue()
	adapter.setPluginStatusSnapshotRows(current)
	go adapter.watch(ctx, current, statusCtr)
}

type pluginStatusAdapter struct {
	producer *BldrDevtoolStatusProducer
}

func (a *pluginStatusAdapter) watch(
	ctx context.Context,
	current *plugin_host_scheduler.PluginStatusSnapshot,
	statusCtr ccontainer.Watchable[*plugin_host_scheduler.PluginStatusSnapshot],
) {
	err := ccontainer.WatchChanges(
		ctx,
		current,
		statusCtr,
		func(snapshot *plugin_host_scheduler.PluginStatusSnapshot) error {
			a.setPluginStatusSnapshotRows(snapshot)
			return nil
		},
		nil,
	)
	if err != nil && ctx.Err() == nil {
		a.setPluginStatusSnapshotRows(nil)
	}
}

func (a *pluginStatusAdapter) setPluginStatusSnapshotRows(
	snapshot *plugin_host_scheduler.PluginStatusSnapshot,
) {
	rows := pluginStatusRows(snapshot)
	a.producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithPluginRows(rows)
	})
}

func pluginStatusRows(
	snapshot *plugin_host_scheduler.PluginStatusSnapshot,
) []BldrDevtoolPluginRow {
	if snapshot == nil || len(snapshot.Plugins) == 0 {
		return nil
	}
	rows := make([]BldrDevtoolPluginRow, 0, len(snapshot.Plugins))
	for _, plugin := range snapshot.Plugins {
		rows = append(rows, pluginStatusRow(plugin))
	}
	slices.SortFunc(rows, func(a, b BldrDevtoolPluginRow) int {
		return strings.Compare(a.ID, b.ID)
	})
	return rows
}

func pluginStatusRow(plugin *bldr_plugin.PluginStatus) BldrDevtoolPluginRow {
	state := pluginStatusRowState(plugin)
	return BldrDevtoolPluginRow{
		ID:          pluginStatusRowID(plugin.GetPluginId(), plugin.GetInstanceKey()),
		PluginID:    plugin.GetPluginId(),
		InstanceKey: plugin.GetInstanceKey(),
		State:       state,
		Summary:     pluginStatusRowSummary(state),
		Error:       plugin.GetLastErrorMessage(),
		LastErrorAt: pluginStatusTimestamp(plugin.GetLastErrorAt()),
	}
}

func pluginStatusRowID(pluginID, instanceKey string) string {
	if instanceKey == "" {
		return "plugin:" + pluginID
	}
	return "plugin:" + pluginID + "/" + instanceKey
}

func pluginStatusRowState(plugin *bldr_plugin.PluginStatus) BldrDevtoolPluginState {
	if plugin.GetLastErrorMessage() != "" {
		return BldrDevtoolPluginStateErrored
	}
	switch plugin.GetState() {
	case bldr_plugin.PluginState_PluginState_REQUESTED:
		return BldrDevtoolPluginStateRequested
	case bldr_plugin.PluginState_PluginState_RUNNING:
		return BldrDevtoolPluginStateRunning
	default:
		return BldrDevtoolPluginStateUnknown
	}
}

func pluginStatusRowSummary(state BldrDevtoolPluginState) string {
	switch state {
	case BldrDevtoolPluginStateRequested:
		return "plugin requested"
	case BldrDevtoolPluginStateRunning:
		return "plugin running"
	case BldrDevtoolPluginStateErrored:
		return "plugin errored"
	default:
		return ""
	}
}

func pluginStatusTimestamp(ts *timestamp.Timestamp) string {
	if ts == nil || ts.CheckValid() != nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}
