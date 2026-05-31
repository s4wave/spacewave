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
	ctrl := &Controller{le: logrus.NewEntry(logrus.New())}
	_, tracker := ctrl.newSubManifestBuilderTracker("child")
	manifestConfig := &bldr_project.ManifestConfig{}
	restartReasons := make(chan string, 2)
	restart := func(reason string) {
		restartReasons <- reason
	}

	resultPromise, err := tracker.setManifestConfig(manifestConfig, restart)
	if err != nil {
		t.Fatalf("set manifest config: %v", err)
	}
	first := newSubManifestTrackerTestResult("bucket-a")
	tracker.mtx.Lock()
	tracker.setResultLocked(first, nil)
	tracker.mtx.Unlock()

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

	sameResultPromise, err := tracker.setManifestConfig(manifestConfig, restart)
	if err != nil {
		t.Fatalf("set same manifest config: %v", err)
	}
	if sameResultPromise != resultPromise {
		t.Fatal("sub-manifest promise container changed across observations")
	}

	second := newSubManifestTrackerTestResult("bucket-b")
	tracker.mtx.Lock()
	tracker.setResultLocked(second, nil)
	tracker.mtx.Unlock()

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

func newSubManifestTrackerTestResult(bucketID string) *bldr_manifest_builder.BuilderResult {
	meta := bldr_manifest.NewManifestMeta("demo-child", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	return bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo-child"),
		&bucket.ObjectRef{BucketId: bucketID},
		bldr_manifest_builder.NewInputManifest(nil, nil),
	)
}
