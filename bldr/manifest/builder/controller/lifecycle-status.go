package bldr_manifest_builder_controller

// ManifestBuilderLifecycleState describes builder controller lifecycle state.
type ManifestBuilderLifecycleState int32

const (
	// ManifestBuilderLifecycleStateQueued means the builder is waiting to run.
	ManifestBuilderLifecycleStateQueued ManifestBuilderLifecycleState = iota
	// ManifestBuilderLifecycleStateRunning means the builder is active.
	ManifestBuilderLifecycleStateRunning
	// ManifestBuilderLifecycleStateDone means the builder has a usable result.
	ManifestBuilderLifecycleStateDone
	// ManifestBuilderLifecycleStateError means the builder failed.
	ManifestBuilderLifecycleStateError
)

// ManifestBuilderLifecycleStatus describes one builder controller status event.
type ManifestBuilderLifecycleStatus struct {
	State                   ManifestBuilderLifecycleState
	CacheHit                bool
	FullRebuild             bool
	HotRebuild              bool
	WatchedFileCount        int
	DependencyRebuildReason string
	Summary                 string
	Error                   string
}

// ManifestBuilderLifecycleSink consumes builder controller status events.
type ManifestBuilderLifecycleSink interface {
	SetManifestBuilderLifecycleStatus(status ManifestBuilderLifecycleStatus)
}
