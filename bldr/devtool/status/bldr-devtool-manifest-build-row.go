package status

// BldrDevtoolManifestBuildRow describes one manifest build status row.
type BldrDevtoolManifestBuildRow struct {
	ID                      string
	BuildTargets            string
	ManifestID              string
	PlatformID              string
	TargetPlatformIDs       string
	BuildType               string
	RemoteID                string
	State                   BldrDevtoolManifestState
	CacheHit                bool
	FullRebuild             bool
	HotRebuild              bool
	WatchedFileCount        int
	DependencyRebuildReason string
	Summary                 string
	Error                   string
}

func bldrDevtoolManifestBuildRowEqual(a, b BldrDevtoolManifestBuildRow) bool {
	return a.ID == b.ID &&
		a.BuildTargets == b.BuildTargets &&
		a.ManifestID == b.ManifestID &&
		a.PlatformID == b.PlatformID &&
		a.TargetPlatformIDs == b.TargetPlatformIDs &&
		a.BuildType == b.BuildType &&
		a.RemoteID == b.RemoteID &&
		a.State == b.State &&
		a.CacheHit == b.CacheHit &&
		a.FullRebuild == b.FullRebuild &&
		a.HotRebuild == b.HotRebuild &&
		a.WatchedFileCount == b.WatchedFileCount &&
		a.DependencyRebuildReason == b.DependencyRebuildReason &&
		a.Summary == b.Summary &&
		a.Error == b.Error
}
