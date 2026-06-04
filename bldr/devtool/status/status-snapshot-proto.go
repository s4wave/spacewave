package status

import (
	"strings"

	"github.com/aperturerobotics/starpc/srpc"
)

// DevtoolStatusWatchService streams Bldr devtool status snapshots.
type DevtoolStatusWatchService struct {
	producer *BldrDevtoolStatusProducer
}

// NewDevtoolStatusWatchService constructs a DevtoolStatusWatchService.
func NewDevtoolStatusWatchService(producer *BldrDevtoolStatusProducer) *DevtoolStatusWatchService {
	return &DevtoolStatusWatchService{producer: producer}
}

// RegisterDevtoolStatusService registers the devtool status watch service.
func RegisterDevtoolStatusService(mux srpc.Mux, producer *BldrDevtoolStatusProducer) error {
	return SRPCRegisterDevtoolStatusService(mux, NewDevtoolStatusWatchService(producer))
}

// WatchDevtoolStatus emits the current status snapshot and subsequent changes.
func (s *DevtoolStatusWatchService) WatchDevtoolStatus(
	_ *WatchDevtoolStatusRequest,
	strm SRPCDevtoolStatusService_WatchDevtoolStatusStream,
) error {
	ctx := strm.Context()
	current := s.producer.GetStatus()
	for {
		if err := strm.Send(&WatchDevtoolStatusResponse{
			Snapshot: BuildDevtoolStatusSnapshot(current),
		}); err != nil {
			return err
		}
		if current.GetCommand().IsTerminal() {
			return nil
		}

		next, err := s.producer.GetStatusCtr().WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		current = next
	}
}

// BuildDevtoolStatusSnapshot converts the Go status projection into its wire shape.
func BuildDevtoolStatusSnapshot(snapshot *BldrDevtoolStatus) *DevtoolStatusSnapshot {
	snapshot = normalizeBldrDevtoolStatus(snapshot)
	return &DevtoolStatusSnapshot{
		Command:           buildDevtoolStatusCommand(snapshot.GetCommand()),
		Project:           buildDevtoolStatusProject(snapshot.GetProject()),
		ManifestFetchRows: buildDevtoolStatusManifestFetchRows(snapshot.GetManifestFetchRows()),
		ManifestBuildRows: buildDevtoolStatusManifestBuildRows(snapshot.GetManifestBuildRows()),
		ControllerRows:    buildDevtoolStatusControllerRows(snapshot.GetControllerRows()),
		PluginRows:        buildDevtoolStatusPluginRows(snapshot.GetPluginRows()),
		AttentionRows:     buildDevtoolStatusAttentionRows(snapshot.GetAttentionRows()),
	}
}

func buildDevtoolStatusCommand(command BldrDevtoolCommandStatus) *DevtoolStatusCommand {
	return &DevtoolStatusCommand{
		Name:    command.Name,
		State:   buildDevtoolStatusCommandState(command.State),
		Summary: command.Summary,
		Error:   command.Error,
		LogFile: command.LogFile,
	}
}

func buildDevtoolStatusCommandState(state BldrDevtoolCommandState) DevtoolStatusCommandState {
	switch state {
	case BldrDevtoolCommandStateStarting:
		return DevtoolStatusCommandState_DevtoolStatusCommandState_STARTING
	case BldrDevtoolCommandStateRunning:
		return DevtoolStatusCommandState_DevtoolStatusCommandState_RUNNING
	case BldrDevtoolCommandStateDone:
		return DevtoolStatusCommandState_DevtoolStatusCommandState_DONE
	case BldrDevtoolCommandStateError:
		return DevtoolStatusCommandState_DevtoolStatusCommandState_ERROR
	case BldrDevtoolCommandStateCanceled:
		return DevtoolStatusCommandState_DevtoolStatusCommandState_CANCELED
	default:
		return DevtoolStatusCommandState_DevtoolStatusCommandState_UNKNOWN
	}
}

func buildDevtoolStatusProject(project BldrDevtoolProjectStatus) *DevtoolStatusProject {
	buildTargets := make([]*DevtoolStatusBuildTarget, 0, len(project.BuildTargets))
	for _, target := range project.BuildTargets {
		buildTargets = append(buildTargets, &DevtoolStatusBuildTarget{
			Id:                  target.ID,
			ManifestIds:         target.ManifestIDs,
			ConfiguredTargetIds: target.ConfiguredTargetIDs,
			ExplicitPlatformIds: target.ExplicitPlatformIDs,
			ResolvedPlatformIds: target.ResolvedPlatformIDs,
			BuildTypes:          target.BuildTypes,
			Error:               target.Error,
		})
	}
	return &DevtoolStatusProject{
		ProjectId:      project.ProjectID,
		StartupPlugins: project.StartupPlugins,
		WebStartupPath: project.WebStartupPath,
		ManifestIds:    project.ManifestIDs,
		BuildTargets:   buildTargets,
	}
}

func buildDevtoolStatusManifestFetchRows(rows []BldrDevtoolManifestFetchRow) []*DevtoolStatusManifestFetchRow {
	out := make([]*DevtoolStatusManifestFetchRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolStatusManifestFetchRow{
			Id:                  row.ID,
			ManifestId:          row.ManifestID,
			PlatformIds:         splitStatusList(row.PlatformID),
			BuildTypes:          splitStatusList(row.BuildType),
			RemoteIds:           splitStatusList(row.RemoteID),
			State:               buildDevtoolStatusManifestState(row.State),
			ReadyRefCount:       uint32(row.ReadyRefCount),
			ReadyRefs:           row.ReadyRefs,
			LocalBuildIds:       splitStatusList(row.LocalBuildIDs),
			BlockedOnLocalBuild: row.BlockedOnLocalBuild,
			Summary:             row.Summary,
			Error:               row.Error,
		})
	}
	return out
}

func buildDevtoolStatusManifestBuildRows(rows []BldrDevtoolManifestBuildRow) []*DevtoolStatusManifestBuildRow {
	out := make([]*DevtoolStatusManifestBuildRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolStatusManifestBuildRow{
			Id:                      row.ID,
			BuildTargetIds:          splitStatusList(row.BuildTargets),
			ManifestId:              row.ManifestID,
			PlatformId:              row.PlatformID,
			TargetPlatformIds:       splitStatusList(row.TargetPlatformIDs),
			BuildType:               row.BuildType,
			RemoteId:                row.RemoteID,
			State:                   buildDevtoolStatusManifestState(row.State),
			CacheHit:                row.CacheHit,
			FullRebuild:             row.FullRebuild,
			HotRebuild:              row.HotRebuild,
			WatchedFileCount:        uint32(row.WatchedFileCount),
			DependencyRebuildReason: row.DependencyRebuildReason,
			Summary:                 row.Summary,
			Error:                   row.Error,
		})
	}
	return out
}

func buildDevtoolStatusManifestState(state BldrDevtoolManifestState) DevtoolStatusManifestState {
	switch state {
	case BldrDevtoolManifestStateQueued:
		return DevtoolStatusManifestState_DevtoolStatusManifestState_QUEUED
	case BldrDevtoolManifestStateRunning:
		return DevtoolStatusManifestState_DevtoolStatusManifestState_RUNNING
	case BldrDevtoolManifestStateReady:
		return DevtoolStatusManifestState_DevtoolStatusManifestState_READY
	case BldrDevtoolManifestStateError:
		return DevtoolStatusManifestState_DevtoolStatusManifestState_ERROR
	case BldrDevtoolManifestStateCanceled:
		return DevtoolStatusManifestState_DevtoolStatusManifestState_CANCELED
	default:
		return DevtoolStatusManifestState_DevtoolStatusManifestState_UNKNOWN
	}
}

func buildDevtoolStatusControllerRows(rows []BldrDevtoolControllerRow) []*DevtoolStatusControllerRow {
	out := make([]*DevtoolStatusControllerRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolStatusControllerRow{
			Id:           row.ID,
			ControllerId: row.ControllerID,
			Kind:         row.Kind,
			State:        buildDevtoolStatusControllerState(row.State),
			Summary:      row.Summary,
			Error:        row.Error,
		})
	}
	return out
}

func buildDevtoolStatusControllerState(state BldrDevtoolControllerState) DevtoolStatusControllerState {
	switch state {
	case BldrDevtoolControllerStateRequested:
		return DevtoolStatusControllerState_DevtoolStatusControllerState_REQUESTED
	case BldrDevtoolControllerStateRunning:
		return DevtoolStatusControllerState_DevtoolStatusControllerState_RUNNING
	case BldrDevtoolControllerStateIdle:
		return DevtoolStatusControllerState_DevtoolStatusControllerState_IDLE
	case BldrDevtoolControllerStateError:
		return DevtoolStatusControllerState_DevtoolStatusControllerState_ERROR
	default:
		return DevtoolStatusControllerState_DevtoolStatusControllerState_UNKNOWN
	}
}

func buildDevtoolStatusPluginRows(rows []BldrDevtoolPluginRow) []*DevtoolStatusPluginRow {
	out := make([]*DevtoolStatusPluginRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolStatusPluginRow{
			Id:          row.ID,
			PluginId:    row.PluginID,
			InstanceKey: row.InstanceKey,
			State:       buildDevtoolStatusPluginState(row.State),
			Summary:     row.Summary,
			Error:       row.Error,
			LastErrorAt: row.LastErrorAt,
		})
	}
	return out
}

func buildDevtoolStatusPluginState(state BldrDevtoolPluginState) DevtoolStatusPluginState {
	switch state {
	case BldrDevtoolPluginStateRequested:
		return DevtoolStatusPluginState_DevtoolStatusPluginState_REQUESTED
	case BldrDevtoolPluginStateRunning:
		return DevtoolStatusPluginState_DevtoolStatusPluginState_RUNNING
	case BldrDevtoolPluginStateErrored:
		return DevtoolStatusPluginState_DevtoolStatusPluginState_ERRORED
	default:
		return DevtoolStatusPluginState_DevtoolStatusPluginState_UNKNOWN
	}
}

func buildDevtoolStatusAttentionRows(rows []BldrDevtoolAttentionRow) []*DevtoolStatusAttentionRow {
	out := make([]*DevtoolStatusAttentionRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &DevtoolStatusAttentionRow{
			Id:       row.ID,
			Source:   row.Source,
			Message:  row.Message,
			Detail:   row.Detail,
			Severity: buildDevtoolStatusAttentionSeverity(row.Severity),
		})
	}
	return out
}

func buildDevtoolStatusAttentionSeverity(
	severity BldrDevtoolAttentionSeverity,
) DevtoolStatusAttentionSeverity {
	switch severity {
	case BldrDevtoolAttentionSeverityInfo:
		return DevtoolStatusAttentionSeverity_DevtoolStatusAttentionSeverity_INFO
	case BldrDevtoolAttentionSeverityWarning:
		return DevtoolStatusAttentionSeverity_DevtoolStatusAttentionSeverity_WARNING
	case BldrDevtoolAttentionSeverityError:
		return DevtoolStatusAttentionSeverity_DevtoolStatusAttentionSeverity_ERROR
	default:
		return DevtoolStatusAttentionSeverity_DevtoolStatusAttentionSeverity_UNKNOWN
	}
}

func splitStatusList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

var _ SRPCDevtoolStatusServiceServer = ((*DevtoolStatusWatchService)(nil))
