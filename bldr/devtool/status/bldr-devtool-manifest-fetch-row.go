package status

import (
	"slices"
	"strings"
)

// BldrDevtoolManifestState describes manifest fetch and build progress.
type BldrDevtoolManifestState int32

const (
	// BldrDevtoolManifestStateUnknown leaves manifest state unset.
	BldrDevtoolManifestStateUnknown BldrDevtoolManifestState = iota
	// BldrDevtoolManifestStateQueued means work is waiting to start.
	BldrDevtoolManifestStateQueued
	// BldrDevtoolManifestStateRunning means work is active.
	BldrDevtoolManifestStateRunning
	// BldrDevtoolManifestStateReady means the manifest is available.
	BldrDevtoolManifestStateReady
	// BldrDevtoolManifestStateError means the work failed.
	BldrDevtoolManifestStateError
	// BldrDevtoolManifestStateCanceled means the work was canceled.
	BldrDevtoolManifestStateCanceled
)

// String returns the stable display value.
func (s BldrDevtoolManifestState) String() string {
	switch s {
	case BldrDevtoolManifestStateQueued:
		return "queued"
	case BldrDevtoolManifestStateRunning:
		return "running"
	case BldrDevtoolManifestStateReady:
		return "ready"
	case BldrDevtoolManifestStateError:
		return "error"
	case BldrDevtoolManifestStateCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// BldrDevtoolManifestFetchRow describes one manifest fetch status row.
type BldrDevtoolManifestFetchRow struct {
	ID                  string
	ManifestID          string
	PlatformID          string
	BuildType           string
	RemoteID            string
	State               BldrDevtoolManifestState
	ReadyRefCount       int
	ReadyRefs           string
	LocalBuildIDs       string
	BlockedOnLocalBuild bool
	Summary             string
	Error               string
}

func bldrDevtoolManifestFetchRowEqual(a, b BldrDevtoolManifestFetchRow) bool {
	return a.ID == b.ID &&
		a.ManifestID == b.ManifestID &&
		a.PlatformID == b.PlatformID &&
		a.BuildType == b.BuildType &&
		a.RemoteID == b.RemoteID &&
		a.State == b.State &&
		a.ReadyRefCount == b.ReadyRefCount &&
		a.ReadyRefs == b.ReadyRefs &&
		a.LocalBuildIDs == b.LocalBuildIDs &&
		a.BlockedOnLocalBuild == b.BlockedOnLocalBuild &&
		a.Summary == b.Summary &&
		a.Error == b.Error
}

func enrichManifestFetchRows(
	rows []BldrDevtoolManifestFetchRow,
	buildRows []BldrDevtoolManifestBuildRow,
) []BldrDevtoolManifestFetchRow {
	enriched := make([]BldrDevtoolManifestFetchRow, len(rows))
	for idx, row := range rows {
		enriched[idx] = enrichManifestFetchRow(row, buildRows)
	}
	return enriched
}

func enrichManifestFetchRow(
	row BldrDevtoolManifestFetchRow,
	buildRows []BldrDevtoolManifestBuildRow,
) BldrDevtoolManifestFetchRow {
	row.RemoteID = ""
	row.LocalBuildIDs = ""
	row.BlockedOnLocalBuild = false

	var buildIDs []string
	var remoteIDs []string
	for _, buildRow := range buildRows {
		if !manifestFetchMatchesBuildRow(row, buildRow) {
			continue
		}
		buildIDs = append(buildIDs, buildRow.ID)
		if buildRow.RemoteID != "" && !slices.Contains(remoteIDs, buildRow.RemoteID) {
			remoteIDs = append(remoteIDs, buildRow.RemoteID)
		}
		if row.ReadyRefCount == 0 &&
			(buildRow.State == BldrDevtoolManifestStateQueued ||
				buildRow.State == BldrDevtoolManifestStateRunning) {
			row.BlockedOnLocalBuild = true
		}
	}

	row.LocalBuildIDs = strings.Join(buildIDs, ",")
	row.RemoteID = strings.Join(remoteIDs, ",")
	return row
}

func manifestFetchMatchesBuildRow(
	fetchRow BldrDevtoolManifestFetchRow,
	buildRow BldrDevtoolManifestBuildRow,
) bool {
	return fetchRow.ManifestID == buildRow.ManifestID &&
		manifestFetchPlatformMatches(fetchRow.PlatformID, buildRow.PlatformID) &&
		manifestFetchBuildTypeMatches(fetchRow.BuildType, buildRow.BuildType)
}

func manifestFetchPlatformMatches(fetchPlatformIDs string, buildPlatformID string) bool {
	for platformID := range strings.SplitSeq(fetchPlatformIDs, ",") {
		if strings.TrimSpace(platformID) == buildPlatformID {
			return true
		}
	}
	return false
}

func manifestFetchBuildTypeMatches(fetchBuildTypes string, buildType string) bool {
	if fetchBuildTypes == "" {
		return buildType == "dev"
	}
	buildTypes := strings.Split(fetchBuildTypes, ",")
	hasDev := false
	hasRelease := false
	for _, rawBuildType := range buildTypes {
		switch strings.TrimSpace(rawBuildType) {
		case "dev":
			hasDev = true
		case "release":
			hasRelease = true
		}
	}
	if hasDev && hasRelease {
		return buildType == "release"
	}
	return strings.TrimSpace(buildTypes[0]) == buildType
}
