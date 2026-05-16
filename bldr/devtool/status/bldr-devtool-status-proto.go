package status

func newWatchDevtoolStatusResponse(snapshot *BldrDevtoolStatus) *WatchDevtoolStatusResponse {
	return &WatchDevtoolStatusResponse{Snapshot: newDevtoolStatusSnapshot(snapshot)}
}

func newDevtoolStatusSnapshot(snapshot *BldrDevtoolStatus) *DevtoolStatusSnapshot {
	snapshot = normalizeBldrDevtoolStatus(snapshot)
	return &DevtoolStatusSnapshot{
		Command:           newDevtoolCommandStatus(snapshot.GetCommand()),
		ManifestFetchRows: newDevtoolManifestFetchRows(snapshot.GetManifestFetchRows()),
		ManifestBuildRows: newDevtoolManifestBuildRows(snapshot.GetManifestBuildRows()),
		PluginRows:        newDevtoolPluginRows(snapshot.GetPluginRows()),
		ControllerRows:    newDevtoolControllerRows(snapshot.GetControllerRows()),
		AttentionRows:     newDevtoolAttentionRows(snapshot.GetAttentionRows()),
	}
}

func newDevtoolCommandStatus(command BldrDevtoolCommandStatus) *DevtoolCommandStatus {
	return &DevtoolCommandStatus{
		Name:    command.Name,
		State:   newDevtoolCommandState(command.State),
		Summary: command.Summary,
		Error:   command.Error,
		LogFile: command.LogFile,
	}
}

func newDevtoolManifestFetchRows(rows []BldrDevtoolManifestFetchRow) []*DevtoolManifestFetchRow {
	out := make([]*DevtoolManifestFetchRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolManifestFetchRow{
			Id:                  row.ID,
			ManifestId:          row.ManifestID,
			PlatformId:          row.PlatformID,
			BuildType:           row.BuildType,
			RemoteId:            row.RemoteID,
			State:               newDevtoolManifestState(row.State),
			ReadyRefCount:       int32(row.ReadyRefCount),
			ReadyRefs:           row.ReadyRefs,
			LocalBuildIds:       row.LocalBuildIDs,
			BlockedOnLocalBuild: row.BlockedOnLocalBuild,
			Summary:             row.Summary,
			Error:               row.Error,
		})
	}
	return out
}

func newDevtoolManifestBuildRows(rows []BldrDevtoolManifestBuildRow) []*DevtoolManifestBuildRow {
	out := make([]*DevtoolManifestBuildRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolManifestBuildRow{
			Id:                      row.ID,
			BuildTargets:            row.BuildTargets,
			ManifestId:              row.ManifestID,
			PlatformId:              row.PlatformID,
			TargetPlatformIds:       row.TargetPlatformIDs,
			BuildType:               row.BuildType,
			RemoteId:                row.RemoteID,
			State:                   newDevtoolManifestState(row.State),
			CacheHit:                row.CacheHit,
			FullRebuild:             row.FullRebuild,
			HotRebuild:              row.HotRebuild,
			WatchedFileCount:        int32(row.WatchedFileCount),
			DependencyRebuildReason: row.DependencyRebuildReason,
			Summary:                 row.Summary,
			Error:                   row.Error,
		})
	}
	return out
}

func newDevtoolPluginRows(rows []BldrDevtoolPluginRow) []*DevtoolPluginRow {
	out := make([]*DevtoolPluginRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolPluginRow{
			Id:          row.ID,
			PluginId:    row.PluginID,
			InstanceKey: row.InstanceKey,
			State:       newDevtoolPluginState(row.State),
			Summary:     row.Summary,
			Error:       row.Error,
			LastErrorAt: row.LastErrorAt,
		})
	}
	return out
}

func newDevtoolControllerRows(rows []BldrDevtoolControllerRow) []*DevtoolControllerRow {
	out := make([]*DevtoolControllerRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolControllerRow{
			Id:           row.ID,
			ControllerId: row.ControllerID,
			Kind:         row.Kind,
			State:        newDevtoolControllerState(row.State),
			Summary:      row.Summary,
			Error:        row.Error,
		})
	}
	return out
}

func newDevtoolAttentionRows(rows []BldrDevtoolAttentionRow) []*DevtoolAttentionRow {
	out := make([]*DevtoolAttentionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolAttentionRow{
			Id:       row.ID,
			Source:   row.Source,
			Message:  row.Message,
			Detail:   row.Detail,
			Severity: newDevtoolAttentionSeverity(row.Severity),
		})
	}
	return out
}

func newDevtoolCommandState(state BldrDevtoolCommandState) DevtoolCommandState {
	switch state {
	case BldrDevtoolCommandStateStarting:
		return DevtoolCommandState_DEVTOOL_COMMAND_STATE_STARTING
	case BldrDevtoolCommandStateRunning:
		return DevtoolCommandState_DEVTOOL_COMMAND_STATE_RUNNING
	case BldrDevtoolCommandStateDone:
		return DevtoolCommandState_DEVTOOL_COMMAND_STATE_DONE
	case BldrDevtoolCommandStateError:
		return DevtoolCommandState_DEVTOOL_COMMAND_STATE_ERROR
	case BldrDevtoolCommandStateCanceled:
		return DevtoolCommandState_DEVTOOL_COMMAND_STATE_CANCELED
	default:
		return DevtoolCommandState_DEVTOOL_COMMAND_STATE_UNSPECIFIED
	}
}

func newDevtoolManifestState(state BldrDevtoolManifestState) DevtoolManifestState {
	switch state {
	case BldrDevtoolManifestStateQueued:
		return DevtoolManifestState_DEVTOOL_MANIFEST_STATE_QUEUED
	case BldrDevtoolManifestStateRunning:
		return DevtoolManifestState_DEVTOOL_MANIFEST_STATE_RUNNING
	case BldrDevtoolManifestStateReady:
		return DevtoolManifestState_DEVTOOL_MANIFEST_STATE_READY
	case BldrDevtoolManifestStateError:
		return DevtoolManifestState_DEVTOOL_MANIFEST_STATE_ERROR
	case BldrDevtoolManifestStateCanceled:
		return DevtoolManifestState_DEVTOOL_MANIFEST_STATE_CANCELED
	default:
		return DevtoolManifestState_DEVTOOL_MANIFEST_STATE_UNSPECIFIED
	}
}

func newDevtoolPluginState(state BldrDevtoolPluginState) DevtoolPluginState {
	switch state {
	case BldrDevtoolPluginStateRequested:
		return DevtoolPluginState_DEVTOOL_PLUGIN_STATE_REQUESTED
	case BldrDevtoolPluginStateRunning:
		return DevtoolPluginState_DEVTOOL_PLUGIN_STATE_RUNNING
	case BldrDevtoolPluginStateErrored:
		return DevtoolPluginState_DEVTOOL_PLUGIN_STATE_ERRORED
	default:
		return DevtoolPluginState_DEVTOOL_PLUGIN_STATE_UNSPECIFIED
	}
}

func newDevtoolControllerState(state BldrDevtoolControllerState) DevtoolControllerState {
	switch state {
	case BldrDevtoolControllerStateRequested:
		return DevtoolControllerState_DEVTOOL_CONTROLLER_STATE_REQUESTED
	case BldrDevtoolControllerStateRunning:
		return DevtoolControllerState_DEVTOOL_CONTROLLER_STATE_RUNNING
	case BldrDevtoolControllerStateIdle:
		return DevtoolControllerState_DEVTOOL_CONTROLLER_STATE_IDLE
	case BldrDevtoolControllerStateError:
		return DevtoolControllerState_DEVTOOL_CONTROLLER_STATE_ERROR
	default:
		return DevtoolControllerState_DEVTOOL_CONTROLLER_STATE_UNSPECIFIED
	}
}

func newDevtoolAttentionSeverity(severity BldrDevtoolAttentionSeverity) DevtoolAttentionSeverity {
	switch severity {
	case BldrDevtoolAttentionSeverityInfo:
		return DevtoolAttentionSeverity_DEVTOOL_ATTENTION_SEVERITY_INFO
	case BldrDevtoolAttentionSeverityWarning:
		return DevtoolAttentionSeverity_DEVTOOL_ATTENTION_SEVERITY_WARNING
	case BldrDevtoolAttentionSeverityError:
		return DevtoolAttentionSeverity_DEVTOOL_ATTENTION_SEVERITY_ERROR
	default:
		return DevtoolAttentionSeverity_DEVTOOL_ATTENTION_SEVERITY_UNSPECIFIED
	}
}
