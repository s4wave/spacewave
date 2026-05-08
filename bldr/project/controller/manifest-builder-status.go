package bldr_project_controller

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
type ManifestBuilderStatus struct {
	ID                      string
	BuildTargetIDs          []string
	ManifestID              string
	PlatformID              string
	TargetPlatformIDs       []string
	BuildType               string
	RemoteID                string
	State                   ManifestBuilderStatusState
	CacheHit                bool
	FullRebuild             bool
	HotRebuild              bool
	WatchedFileCount        int
	DependencyRebuildReason string
	Summary                 string
	Error                   string
}

// ManifestBuilderStatusSink consumes ProjectController manifest build status.
type ManifestBuilderStatusSink interface {
	SetManifestBuilderStatus(status ManifestBuilderStatus)
}
