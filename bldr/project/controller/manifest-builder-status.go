package bldr_project_controller

import (
	"slices"
	"strings"
	"sync"

	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

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
// by the devtool TUI. Identity and target fields are set at construction, and
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
	// BuildType is the build type.
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
	// Error contains the last build error, if any.
	Error string
}

// ManifestBuilderStatusSink consumes ProjectController manifest build status.
type ManifestBuilderStatusSink interface {
	// SetManifestBuilderStatus receives the latest status of one builder.
	SetManifestBuilderStatus(status ManifestBuilderStatus)
}

// ProjectConfigStatusSink consumes ProjectController project config status.
type ProjectConfigStatusSink interface {
	// SetProjectConfigStatus receives a detached copy of the current config.
	SetProjectConfigStatus(projectConfig *bldr_project.ProjectConfig)
}

// statusPublisher holds the status sinks and the build targets each manifest
// builder serves, and publishes status to the sinks. The zero value is ready
// to use.
type statusPublisher struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// manifestBuilderSink receives manifest build status events.
	manifestBuilderSink ManifestBuilderStatusSink
	// projectConfigSink receives project config status events.
	projectConfigSink ProjectConfigStatusSink
	// buildTargets records finite build targets by builder key.
	buildTargets map[string][]string
}

// setManifestBuilderSink replaces the manifest builder status sink.
func (p *statusPublisher) setManifestBuilderSink(sink ManifestBuilderStatusSink) {
	p.mtx.Lock()
	p.manifestBuilderSink = sink
	p.mtx.Unlock()
}

// setProjectConfigSink replaces the project config status sink.
func (p *statusPublisher) setProjectConfigSink(sink ProjectConfigStatusSink) {
	p.mtx.Lock()
	p.projectConfigSink = sink
	p.mtx.Unlock()
}

// publishManifestBuilderStatus passes status to the sink when one is attached.
// The sink is called outside the lock so it can replace itself.
func (p *statusPublisher) publishManifestBuilderStatus(status ManifestBuilderStatus) {
	p.mtx.Lock()
	sink := p.manifestBuilderSink
	p.mtx.Unlock()
	if sink != nil {
		sink.SetManifestBuilderStatus(status)
	}
}

// publishProjectConfigStatus passes a clone of the project config to the sink
// when one is attached.
func (p *statusPublisher) publishProjectConfigStatus(projectConfig *bldr_project.ProjectConfig) {
	p.mtx.Lock()
	sink := p.projectConfigSink
	p.mtx.Unlock()
	if sink != nil {
		sink.SetProjectConfigStatus(projectConfig.CloneVT())
	}
}

// addBuildTarget records a build target name for a manifest builder config,
// ignoring an empty name and a repeat.
func (p *statusPublisher) addBuildTarget(conf *ManifestBuilderConfig, target string) {
	// Ignore empty target names.
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}

	// Record the target under the config key.
	key := conf.MarshalB58()
	p.mtx.Lock()
	if p.buildTargets == nil {
		p.buildTargets = make(map[string][]string)
	}
	if !slices.Contains(p.buildTargets[key], target) {
		p.buildTargets[key] = append(p.buildTargets[key], target)
	}
	p.mtx.Unlock()
}

// getBuildTargets returns the recorded build targets for a manifest builder
// config key.
func (p *statusPublisher) getBuildTargets(key string) []string {
	p.mtx.Lock()
	targets := slices.Clone(p.buildTargets[key])
	p.mtx.Unlock()
	return targets
}
