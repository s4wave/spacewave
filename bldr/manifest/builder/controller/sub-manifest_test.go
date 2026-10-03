//go:build !js

package bldr_manifest_builder_controller

import (
	"context"
	"testing"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

func TestSubManifestTrackerPublishesResultsThroughStablePromiseContainer(t *testing.T) {
	// Create a child Manifest tracker and capture parent restart requests.
	ctrl := &Controller{le: logrus.NewEntry(logrus.New())}
	_, tracker := ctrl.newSubManifestBuilderTracker("child")
	manifestConfig := &bldr_project.ManifestConfig{}
	restartReasons := make(chan string, 2)
	restart := func(reason string) {
		restartReasons <- reason
	}

	// Publish the first child Manifest result through its promise container.
	resultPromise, err := tracker.setManifestConfig(manifestConfig, restart)
	if err != nil {
		t.Fatalf("set manifest config: %v", err)
	}
	first := newSubManifestTrackerTestResult("bucket-a")
	tracker.build.setResult(first, nil)

	// Verify the first result arrives without requesting a parent restart.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := resultPromise.Await(ctx)
	if err != nil {
		t.Fatalf("await first result: %v", err)
	}
	if got.GetManifestRef().GetManifestRef().GetBucketId() != "bucket-a" {
		t.Fatalf("first bucket = %q, want bucket-a", got.GetManifestRef().GetManifestRef().GetBucketId())
	}
	select {
	case reason := <-restartReasons:
		t.Fatalf("unexpected restart before observed result changes: %q", reason)
	default:
	}

	// Require repeated child observations to retain the same promise container.
	sameResultPromise, err := tracker.setManifestConfig(manifestConfig, restart)
	if err != nil {
		t.Fatalf("set same manifest config: %v", err)
	}
	if sameResultPromise != resultPromise {
		t.Fatal("sub-manifest promise container changed across observations")
	}

	// Publish a changed child Manifest result after the parent observes it.
	second := newSubManifestTrackerTestResult("bucket-b")
	tracker.build.setResult(second, nil)

	// Verify the parent restarts once and the stable promise yields the new result.
	select {
	case reason := <-restartReasons:
		if reason != "sub-manifest changed: child" {
			t.Fatalf("restart reason = %q, want child sub-manifest change", reason)
		}
	case <-ctx.Done():
		t.Fatal("expected sub-manifest result change to request parent restart")
	}
	got, err = resultPromise.Await(ctx)
	if err != nil {
		t.Fatalf("await second result: %v", err)
	}
	if got.GetManifestRef().GetManifestRef().GetBucketId() != "bucket-b" {
		t.Fatalf("second bucket = %q, want bucket-b", got.GetManifestRef().GetManifestRef().GetBucketId())
	}
	select {
	case reason := <-restartReasons:
		t.Fatalf("unexpected duplicate restart reason: %q", reason)
	default:
	}
}

func TestSubManifestBuildOwnerParentAttemptObservationLifecycle(t *testing.T) {
	// Create a child Manifest tracker with parent restart notifications.
	ctrl := &Controller{le: logrus.NewEntry(logrus.New())}
	_, tracker := ctrl.newSubManifestBuilderTracker("child")
	manifestConfig := &bldr_project.ManifestConfig{}
	restartReasons := make(chan string, 2)
	restart := func(reason string) {
		restartReasons <- reason
	}

	// Publish and observe the child result during the first parent attempt.
	resultPromise, err := tracker.setManifestConfig(manifestConfig, restart)
	if err != nil {
		t.Fatalf("set manifest config: %v", err)
	}
	tracker.build.setResult(newSubManifestTrackerTestResult("bucket-a"), nil)
	if !tracker.build.observedInParentAttempt() {
		t.Fatal("sub-manifest should be observed after BuildSubManifest returns its promise")
	}

	// Begin a new parent attempt and change the unobserved child result.
	tracker.build.prepareParentAttempt(restart)
	if tracker.build.observedInParentAttempt() {
		t.Fatal("new parent attempt should start with child unobserved")
	}
	tracker.build.setResult(newSubManifestTrackerTestResult("bucket-b"), nil)
	select {
	case reason := <-restartReasons:
		t.Fatalf("unexpected restart for unobserved child result change: %q", reason)
	default:
	}

	// Verify the stable promise exposes the latest unobserved child result.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, err := resultPromise.Await(ctx)
	if err != nil {
		t.Fatalf("await unobserved child result: %v", err)
	}
	if got.GetManifestRef().GetManifestRef().GetBucketId() != "bucket-b" {
		t.Fatalf("unobserved child bucket = %q, want bucket-b", got.GetManifestRef().GetManifestRef().GetBucketId())
	}

	// Observe the child again and require its next change to restart the parent.
	if _, err := tracker.setManifestConfig(manifestConfig, restart); err != nil {
		t.Fatalf("set same manifest config: %v", err)
	}
	if !tracker.build.observedInParentAttempt() {
		t.Fatal("sub-manifest should be observed after parent calls BuildSubManifest")
	}
	tracker.build.setResult(newSubManifestTrackerTestResult("bucket-c"), nil)
	select {
	case reason := <-restartReasons:
		if reason != "sub-manifest changed: child" {
			t.Fatalf("restart reason = %q, want child sub-manifest change", reason)
		}
	case <-ctx.Done():
		t.Fatal("expected observed child result change to request parent restart")
	}
	if tracker.build.observedInParentAttempt() {
		t.Fatal("child should become unobserved after its result changes and restarts the parent")
	}

	// Require later unobserved child changes to avoid duplicate parent restarts.
	tracker.build.setResult(newSubManifestTrackerTestResult("bucket-d"), nil)
	select {
	case reason := <-restartReasons:
		t.Fatalf("unexpected duplicate restart while child is unobserved: %q", reason)
	default:
	}
}

func newSubManifestTrackerTestResult(bucketID string) *bldr_manifest_builder.BuilderResult {
	meta := bldr_manifest.NewManifestMeta("demo-child", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	return bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo-child"),
		&bucket.ObjectRef{BucketId: bucketID},
		bldr_manifest_builder.NewInputManifest(nil, nil),
	)
}
