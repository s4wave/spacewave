//go:build !js

package status

import (
	"slices"
	"strings"

	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
)

// AttachManifestBuildStatus adapts ProjectController builder status into Bldr Devtool Status.
func AttachManifestBuildStatus(
	producer *BldrDevtoolStatusProducer,
	ctrl *bldr_project_controller.Controller,
) {
	ctrl.SetManifestBuilderStatusSink(&manifestBuildStatusAdapter{
		producer: producer,
	})
}

type manifestBuildStatusAdapter struct {
	producer *BldrDevtoolStatusProducer
}

func (a *manifestBuildStatusAdapter) SetManifestBuilderStatus(
	status bldr_project_controller.ManifestBuilderStatus,
) {
	row := manifestBuildStatusRow(status)
	a.producer.UpdateStatus(func(current *BldrDevtoolStatus) *BldrDevtoolStatus {
		rows := current.GetManifestBuildRows()
		replaced := false
		for idx, existing := range rows {
			if existing.ID != row.ID {
				continue
			}
			rows[idx] = row
			replaced = true
			break
		}
		if !replaced {
			rows = append(rows, row)
		}
		slices.SortFunc(rows, func(a, b BldrDevtoolManifestBuildRow) int {
			return strings.Compare(a.ID, b.ID)
		})
		return current.WithManifestBuildRows(rows)
	})
}

func manifestBuildStatusRow(
	status bldr_project_controller.ManifestBuilderStatus,
) BldrDevtoolManifestBuildRow {
	return BldrDevtoolManifestBuildRow{
		ID:                      status.ID,
		BuildTargets:            strings.Join(status.BuildTargetIDs, ","),
		ManifestID:              status.ManifestID,
		PlatformID:              status.PlatformID,
		TargetPlatformIDs:       strings.Join(status.TargetPlatformIDs, ","),
		BuildType:               status.BuildType,
		RemoteID:                status.RemoteID,
		State:                   manifestBuildStatusState(status.State),
		CacheHit:                status.CacheHit,
		FullRebuild:             status.FullRebuild,
		HotRebuild:              status.HotRebuild,
		WatchedFileCount:        status.WatchedFileCount,
		DependencyRebuildReason: status.DependencyRebuildReason,
		Summary:                 status.Summary,
		Error:                   status.Error,
	}
}

func manifestBuildStatusState(
	state bldr_project_controller.ManifestBuilderStatusState,
) BldrDevtoolManifestState {
	switch state {
	case bldr_project_controller.ManifestBuilderStatusStateQueued:
		return BldrDevtoolManifestStateQueued
	case bldr_project_controller.ManifestBuilderStatusStateRunning:
		return BldrDevtoolManifestStateRunning
	case bldr_project_controller.ManifestBuilderStatusStateDone:
		return BldrDevtoolManifestStateReady
	case bldr_project_controller.ManifestBuilderStatusStateError:
		return BldrDevtoolManifestStateError
	case bldr_project_controller.ManifestBuilderStatusStateCanceled:
		return BldrDevtoolManifestStateCanceled
	default:
		return BldrDevtoolManifestStateUnknown
	}
}

// _ is a type assertion
var _ bldr_project_controller.ManifestBuilderStatusSink = ((*manifestBuildStatusAdapter)(nil))
