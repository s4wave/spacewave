//go:build !js

package bldr_manifest_builder_controller

import "testing"

func TestControllerLifecycleStatusReplayAndRebuildMetadata(t *testing.T) {
	ctrl := &Controller{}

	ctrl.setLifecycleStatus(ManifestBuilderLifecycleStatus{
		State:    ManifestBuilderLifecycleStateDone,
		CacheHit: true,
		Summary:  "startup cache hit",
	})

	sink := &recordingLifecycleSink{}
	ctrl.SetManifestBuilderLifecycleSink(sink)
	cacheHit := sink.last(t)
	if cacheHit.State != ManifestBuilderLifecycleStateDone || !cacheHit.CacheHit || cacheHit.Summary != "startup cache hit" {
		t.Fatalf("unexpected replayed cache-hit status: %#v", cacheHit)
	}

	ctrl.setLifecycleStatus(ManifestBuilderLifecycleStatus{
		State:       ManifestBuilderLifecycleStateRunning,
		FullRebuild: true,
		Summary:     rebuildSummary(true, false),
	})
	fullRebuild := sink.last(t)
	if fullRebuild.State != ManifestBuilderLifecycleStateRunning || !fullRebuild.FullRebuild || fullRebuild.HotRebuild {
		t.Fatalf("unexpected full rebuild status: %#v", fullRebuild)
	}
	if fullRebuild.Summary != "full rebuild" {
		t.Fatalf("unexpected full rebuild summary: %q", fullRebuild.Summary)
	}

	ctrl.setLifecycleStatus(ManifestBuilderLifecycleStatus{
		State:                   ManifestBuilderLifecycleStateRunning,
		HotRebuild:              true,
		WatchedFileCount:        4,
		DependencyRebuildReason: "manifest dependency changed: web",
		Summary:                 rebuildSummary(false, true),
	})
	hotRebuild := sink.last(t)
	if hotRebuild.State != ManifestBuilderLifecycleStateRunning || !hotRebuild.HotRebuild || hotRebuild.FullRebuild {
		t.Fatalf("unexpected hot rebuild status: %#v", hotRebuild)
	}
	if hotRebuild.DependencyRebuildReason != "manifest dependency changed: web" || hotRebuild.WatchedFileCount != 4 {
		t.Fatalf("unexpected dependency rebuild metadata: %#v", hotRebuild)
	}
}

func TestRebuildStatusSummaries(t *testing.T) {
	if got := rebuildSummary(true, false); got != "full rebuild" {
		t.Fatalf("unexpected full rebuild summary: %q", got)
	}
	if got := rebuildSummary(false, true); got != "hot rebuild" {
		t.Fatalf("unexpected hot rebuild summary: %q", got)
	}
	if got := changedFilesSummary(0); got != "filesystem change" {
		t.Fatalf("unexpected zero-file change summary: %q", got)
	}
	if got := changedFilesSummary(1); got != "filesystem change: 1 changed file" {
		t.Fatalf("unexpected one-file change summary: %q", got)
	}
	if got := changedFilesSummary(2); got != "filesystem change: multiple changed files" {
		t.Fatalf("unexpected multi-file change summary: %q", got)
	}
}

type recordingLifecycleSink struct {
	statuses []ManifestBuilderLifecycleStatus
}

func (s *recordingLifecycleSink) SetManifestBuilderLifecycleStatus(status ManifestBuilderLifecycleStatus) {
	s.statuses = append(s.statuses, status)
}

func (s *recordingLifecycleSink) last(t *testing.T) ManifestBuilderLifecycleStatus {
	t.Helper()
	if len(s.statuses) == 0 {
		t.Fatal("expected recorded lifecycle status")
	}
	return s.statuses[len(s.statuses)-1]
}
