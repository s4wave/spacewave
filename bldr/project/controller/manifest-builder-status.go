package bldr_project_controller

import bldr_project "github.com/s4wave/spacewave/bldr/project"

// ManifestBuilderStatusState describes ProjectController manifest build status.
type ManifestBuilderStatusState int32

const (
	// ManifestBuilderStatusStateQueued means the manifest build is waiting.
	ManifestBuilderStatusStateQueued ManifestBuilderStatusState = iota
	// ManifestBuilderStatusStateRunning means the manifest build is active.
	ManifestBuilderStatusStateRunning
	// ManifestBuilderStatusStateDone means the manifest build has a result.
	ManifestBuilderStatusStateDone
	// ManifestBuilderStatusStateError means the manifest build failed.
	ManifestBuilderStatusStateError
	// ManifestBuilderStatusStateCanceled means the manifest build was canceled.
	ManifestBuilderStatusStateCanceled
)

// ManifestBuilderStatus is the ProjectController manifest build status surface.
// ManifestBuilderStatus is the user-visible manifest build status consumed
// by the devtool TUI. Identity and target fields are set at construction;
// cache, rebuild, and watch fields are merged in by lifecycle events.
type ManifestBuilderStatus struct {
	// ID is the manifest builder tracker id.
	ID string
	// BuildTargetIDs lists the build targets this builder serves.
	BuildTargetIDs []string
	// ManifestID is the manifest id under construction.
	ManifestID string
	// PlatformID is the target platform id.
	PlatformID string
	// TargetPlatformIDs lists the platforms the build produces.
	TargetPlatformIDs []string
	// BuildType names the build type.
	BuildType string
	// RemoteID is the publish remote id.
	RemoteID string
	// State is the current lifecycle state.
	State ManifestBuilderStatusState
	// CacheHit reports whether the build reused a startup cache hit.
	CacheHit bool
	// FullRebuild reports whether a full rebuild was required.
	FullRebuild bool
	// HotRebuild reports whether a hot rebuild was used.
	HotRebuild bool
	// WatchedFileCount is the number of files watched for changes.
	WatchedFileCount int
	// DependencyRebuildReason explains why dependencies forced a rebuild.
	DependencyRebuildReason string
	// Summary is a human-readable status line.
	Summary string
	// Error carries the last build error, if any.
	Error string
}

// ManifestBuilderStatusSink consumes ProjectController manifest build status.
type ManifestBuilderStatusSink interface {
	SetManifestBuilderStatus(status ManifestBuilderStatus)
}

// ProjectConfigStatusSink consumes ProjectController project config status.
type ProjectConfigStatusSink interface {
	SetProjectConfigStatus(projectConfig *bldr_project.ProjectConfig)
}
