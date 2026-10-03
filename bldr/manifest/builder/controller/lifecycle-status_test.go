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
	// Create a Manifest builder controller for lifecycle replay.
	ctrl := &Controller{}

	// Retain a completed startup-cache result before attaching a lifecycle sink.
	ctrl.setLifecycleStatus(ManifestBuilderLifecycleStatus{
		State:    ManifestBuilderLifecycleStateDone,
		CacheHit: true,
		Summary:  "startup cache hit",
	})

	// Attach the lifecycle sink and verify it receives the retained cache result.
	sink := &recordingLifecycleSink{}
	ctrl.SetManifestBuilderLifecycleSink(sink)
	cacheHit := sink.last(t)
	if cacheHit.State != ManifestBuilderLifecycleStateDone || !cacheHit.CacheHit || cacheHit.Summary != "startup cache hit" {
		t.Fatalf("unexpected replayed cache-hit status: %#v", cacheHit)
	}

	// Publish a full rebuild and verify its lifecycle metadata.
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

	// Publish a hot rebuild and verify its dependency and watched-file metadata.
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
	// Verify the full and hot rebuild summaries describe their rebuild modes.
	if got := rebuildSummary(true, false); got != "full rebuild" {
		t.Fatalf("unexpected full rebuild summary: %q", got)
	}
	if got := rebuildSummary(false, true); got != "hot rebuild" {
		t.Fatalf("unexpected hot rebuild summary: %q", got)
	}

	// Verify filesystem change summaries distinguish zero, one, and several files.
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
	// Record the lifecycle status and snapshot its notification channel under lock.
	s.mtx.Lock()
	s.statuses = append(s.statuses, status)
	notifyCh := s.notifyCh
	s.mtx.Unlock()

	// Notify lifecycle waiters after releasing the status lock.
	if notifyCh != nil {
		select {
		case notifyCh <- struct{}{}:
		default:
		}
	}
}

func (s *recordingLifecycleSink) last(t *testing.T) ManifestBuilderLifecycleStatus {
	// Require a recorded lifecycle status before returning the latest entry.
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
