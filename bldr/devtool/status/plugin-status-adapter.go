package status

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// AttachPluginStatus adapts PluginHost scheduler status into Bldr Devtool Status.
func AttachPluginStatus(
	ctx context.Context,
	producer *BldrDevtoolStatusProducer,
	ctrl bldr_plugin.PluginScheduler,
) {
	// Publish the current scheduler snapshot and watch for changes.
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
	current *bldr_plugin.PluginStatusSnapshot,
	statusCtr ccontainer.Watchable[*bldr_plugin.PluginStatusSnapshot],
) {
	_ = ccontainer.WatchChanges(
		ctx,
		current,
		statusCtr,
		func(snapshot *bldr_plugin.PluginStatusSnapshot) error {
			a.setPluginStatusSnapshotRows(snapshot)
			return nil
		},
		nil,
	)
}

func (a *pluginStatusAdapter) setPluginStatusSnapshotRows(
	snapshot *bldr_plugin.PluginStatusSnapshot,
) {
	// Replace the plugin rows in the status snapshot.
	rows := pluginStatusRows(snapshot)
	a.producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		return current.WithPluginRows(rows)
	})
}

func pluginStatusRows(
	snapshot *bldr_plugin.PluginStatusSnapshot,
) []BldrDevtoolPluginRow {
	// An empty or missing snapshot clears the plugin rows.
	if snapshot == nil || len(snapshot.Plugins) == 0 {
		return nil
	}

	// Index each execution's swap by its row id.
	swaps := make(map[string]*bldr_plugin.PluginSwap, len(snapshot.GetExecutions()))
	for _, exec := range snapshot.GetExecutions() {
		swaps[pluginStatusRowID(exec.GetPluginId(), exec.GetInstanceKey())] = exec.GetSwap()
	}

	// Build one row per plugin and sort them by row id.
	rows := make([]BldrDevtoolPluginRow, 0, len(snapshot.Plugins))
	for _, plugin := range snapshot.Plugins {
		row := pluginStatusRow(plugin)
		if summary := pluginSwapSummary(swaps[row.ID]); summary != "" {
			row.Summary = summary
		}
		rows = append(rows, row)
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

// pluginSwapSummary describes a swap that is not settled: the candidate rev
// being prepared, waiting on the running execution, having its approval on
// this device checked, awaiting that approval, or failed with its retry. It
// returns an empty string for a settled or missing swap.
func pluginSwapSummary(swap *bldr_plugin.PluginSwap) string {
	// Describe the candidate in the words of its swap state.
	switch st := swap.GetState().(type) {
	case *bldr_plugin.PluginSwap_Preparing:
		return "preparing rev " + strconv.FormatUint(st.Preparing.GetCandidateManifestRev(), 10) +
			" since " + pluginStatusTimestamp(st.Preparing.GetStartedAt())
	case *bldr_plugin.PluginSwap_Waiting:
		return "rev " + strconv.FormatUint(st.Waiting.GetCandidateManifestRev(), 10) +
			" waits since " + pluginStatusTimestamp(st.Waiting.GetSinceAt()) +
			" for the running plugin to answer its activation check"
	case *bldr_plugin.PluginSwap_CheckingApproval:
		return "checking approval of rev " + strconv.FormatUint(st.CheckingApproval.GetCandidateManifestRev(), 10) +
			" on this device since " + pluginStatusTimestamp(st.CheckingApproval.GetSinceAt())
	case *bldr_plugin.PluginSwap_AwaitingApproval:
		return "rev " + strconv.FormatUint(st.AwaitingApproval.GetCandidateManifestRev(), 10) +
			" awaits approval on this device since " + pluginStatusTimestamp(st.AwaitingApproval.GetSinceAt())
	case *bldr_plugin.PluginSwap_Failed:
		next := "retries on the next deploy or host change"
		if st.Failed.GetRetryAt() != nil {
			next = "retry at " + pluginStatusTimestamp(st.Failed.GetRetryAt())
		}
		return "rev " + strconv.FormatUint(st.Failed.GetCandidateManifestRev(), 10) +
			" failed: " + st.Failed.GetErrorMessage() + "; " + next
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
