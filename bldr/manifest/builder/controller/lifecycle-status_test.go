//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

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
	mtx      sync.Mutex
	statuses []ManifestBuilderLifecycleStatus
	notifyCh chan struct{}
}

func (s *recordingLifecycleSink) SetManifestBuilderLifecycleStatus(status ManifestBuilderLifecycleStatus) {
	s.mtx.Lock()
	s.statuses = append(s.statuses, status)
	notifyCh := s.notifyCh
	s.mtx.Unlock()

	if notifyCh != nil {
		select {
		case notifyCh <- struct{}{}:
		default:
		}
	}
}

func (s *recordingLifecycleSink) last(t *testing.T) ManifestBuilderLifecycleStatus {
	t.Helper()
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if len(s.statuses) == 0 {
		t.Fatal("expected recorded lifecycle status")
	}
	return s.statuses[len(s.statuses)-1]
}

func newRecordingLifecycleSink() *recordingLifecycleSink {
	return &recordingLifecycleSink{notifyCh: make(chan struct{}, 32)}
}

func (s *recordingLifecycleSink) snapshot() []ManifestBuilderLifecycleStatus {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return slices.Clone(s.statuses)
}

func (s *recordingLifecycleSink) nonEmptySnapshot() []ManifestBuilderLifecycleStatus {
	statuses := s.snapshot()
	filtered := statuses[:0]
	for _, status := range statuses {
		if status.Summary == "" && status.Error == "" &&
			status.State == ManifestBuilderLifecycleStateQueued &&
			!status.CacheHit && !status.FullRebuild && !status.HotRebuild &&
			status.WatchedFileCount == 0 && status.DependencyRebuildReason == "" {
			continue
		}
		filtered = append(filtered, status)
	}
	return filtered
}

func (s *recordingLifecycleSink) waitFor(
	t *testing.T,
	ctx context.Context,
	match func(ManifestBuilderLifecycleStatus) bool,
) ManifestBuilderLifecycleStatus {
	t.Helper()

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		for _, status := range s.nonEmptySnapshot() {
			if match(status) {
				return status
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context canceled before lifecycle status: %v", ctx.Err())
		case <-timeout.C:
			t.Fatal("timed out waiting for lifecycle status")
		case <-s.notifyCh:
		}
	}
}
