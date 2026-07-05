//go:build !js

package bldr_project_controller

import (
	"bytes"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/util/enabled"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_build "github.com/s4wave/spacewave/bldr/manifest/build"
	manifest_builder_controller "github.com/s4wave/spacewave/bldr/manifest/builder/controller"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
)

func TestApplyBuilderConfigOverride_Replace(t *testing.T) {
	mc := &bldr_project.ManifestConfig{
		Builder: &configset_proto.ControllerConfig{
			Id:     "bldr/plugin/compiler/dist",
			Rev:    1,
			Config: []byte(`{"embedManifests":[{"manifestId":"spacewave-launcher","platformId":"desktop/linux/amd64"}]}`),
		},
	}
	override := &configset_proto.ControllerConfig{
		Id:     "ignored-id",
		Config: []byte(`{"embedManifests":[{"manifestId":"spacewave-launcher","platformId":"desktop/darwin/arm64"}]}`),
	}

	if err := applyBuilderConfigOverride(mc, "spacewave-dist", override); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if mc.GetBuilder().GetId() != "bldr/plugin/compiler/dist" {
		t.Fatalf("builder id should be preserved, got %q", mc.GetBuilder().GetId())
	}
	if !bytes.Equal(mc.GetBuilder().GetConfig(), override.GetConfig()) {
		t.Fatalf("builder config not replaced; got %s", mc.GetBuilder().GetConfig())
	}
	if mc.GetBuilder().GetRev() != 1 {
		t.Fatalf("builder rev should be preserved when override rev is 0, got %d", mc.GetBuilder().GetRev())
	}
}

func TestApplyBuilderConfigOverride_BumpsRev(t *testing.T) {
	mc := &bldr_project.ManifestConfig{
		Builder: &configset_proto.ControllerConfig{
			Id:     "bldr/plugin/compiler/dist",
			Rev:    1,
			Config: []byte(`{}`),
		},
	}
	override := &configset_proto.ControllerConfig{
		Rev:    7,
		Config: []byte(`{"embedManifests":[]}`),
	}

	if err := applyBuilderConfigOverride(mc, "spacewave-dist", override); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if mc.GetBuilder().GetRev() != 7 {
		t.Fatalf("expected rev 7, got %d", mc.GetBuilder().GetRev())
	}
}

func TestApplyBuilderConfigOverride_NilOverride(t *testing.T) {
	mc := &bldr_project.ManifestConfig{
		Builder: &configset_proto.ControllerConfig{
			Id:     "bldr/plugin/compiler/dist",
			Config: []byte(`original`),
		},
	}
	if err := applyBuilderConfigOverride(mc, "spacewave-dist", nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if string(mc.GetBuilder().GetConfig()) != "original" {
		t.Fatalf("nil override should not modify config, got %s", mc.GetBuilder().GetConfig())
	}
}

func TestApplyBuilderConfigOverride_EmptyOverride(t *testing.T) {
	mc := &bldr_project.ManifestConfig{
		Builder: &configset_proto.ControllerConfig{
			Id:     "bldr/plugin/compiler/dist",
			Config: []byte(`original`),
		},
	}
	override := &configset_proto.ControllerConfig{}
	if err := applyBuilderConfigOverride(mc, "spacewave-dist", override); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if string(mc.GetBuilder().GetConfig()) != "original" {
		t.Fatalf("empty override should not modify config, got %s", mc.GetBuilder().GetConfig())
	}
}

func TestApplyBuilderConfigOverride_NoBuilder(t *testing.T) {
	mc := &bldr_project.ManifestConfig{}
	override := &configset_proto.ControllerConfig{
		Config: []byte(`{}`),
	}
	err := applyBuilderConfigOverride(mc, "spacewave-dist", override)
	if err == nil {
		t.Fatal("expected error when manifest has no builder")
	}
}

// TestBuildTargetsOverrideSelection exercises BuildTargets wiring: a
// ManifestBuilderConfig is produced with BuilderConfigOverride populated from
// BuildConfig.ManifestOverrides for the matching manifest id. This proves the
// starlark -> BuildConfig.ManifestOverrides -> ManifestBuilderConfig.
// BuilderConfigOverride path (IC-1 + IC-2) without standing up a live
// controller bus.
func TestBuildTargetsOverrideSelection(t *testing.T) {
	platformIDs := []string{"desktop/darwin/arm64"}
	override := &configset_proto.ControllerConfig{
		Config: []byte(`{"embedManifests":[{"manifestId":"spacewave-launcher","platformId":"desktop/darwin/arm64"}]}`),
	}
	manifestOverrides := map[string]*configset_proto.ControllerConfig{
		"spacewave-dist": override,
	}
	buildPolicy := manifest_build.NewBuildPolicy(enabled.Enabled_DISABLE, enabled.Enabled_ENABLE, enabled.Enabled_ENABLE)

	var gotOverride *configset_proto.ControllerConfig
	var gotPolicy *manifest_build.BuildPolicy
	err := ForManifestSelector(
		[]string{"spacewave-dist", "spacewave-launcher"},
		platformIDs,
		func(manifestID, platformID string) (bool, error) {
			mbc := newBuildTargetManifestBuilderConfig(
				manifestID,
				platformID,
				"devtool",
				bldr_manifest.BuildType_RELEASE,
				platformIDs,
				buildPolicy,
				manifestOverrides,
			)
			if manifestID == "spacewave-dist" {
				gotOverride = mbc.GetBuilderConfigOverride()
			} else if mbc.GetBuilderConfigOverride() != nil {
				t.Fatalf("unexpected override for manifest %s", manifestID)
			}
			gotPolicy = mbc.GetBuildPolicy()
			return true, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotOverride == nil {
		t.Fatal("expected override on spacewave-dist slot")
	}
	if !bytes.Equal(gotOverride.GetConfig(), override.GetConfig()) {
		t.Fatalf("override config mismatch: got %s", gotOverride.GetConfig())
	}
	// CloneVT must decouple the override from the source map.
	if gotOverride == override {
		t.Fatal("override should be cloned, not aliased")
	}
	if gotPolicy.GetJsMinification() != enabled.Enabled_DISABLE {
		t.Fatalf("js_minification: got %s, want DISABLE", gotPolicy.GetJsMinification())
	}
	if gotPolicy.GetGoscriptCodeSplitting() != enabled.Enabled_ENABLE {
		t.Fatalf("goscript_code_splitting: got %s, want ENABLE", gotPolicy.GetGoscriptCodeSplitting())
	}
	if gotPolicy == buildPolicy {
		t.Fatal("build policy should be cloned, not aliased")
	}
}

func TestResolveBuildTargetMergesBuildPolicy(t *testing.T) {
	buildTarget := &bldr_project.BuildConfig{
		Targets: []string{"browser"},
		BuildPolicy: manifest_build.NewBuildPolicy(
			enabled.Enabled_ENABLE,
			enabled.Enabled_DISABLE,
			enabled.Enabled_DISABLE,
		),
	}
	override := manifest_build.NewBuildPolicy(
		enabled.Enabled_DISABLE,
		enabled.Enabled_DEFAULT,
		enabled.Enabled_ENABLE,
	)

	resolved, err := ResolveBuildTarget(buildTarget, nil, override)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.BuildPolicy.GetJsMinification() != enabled.Enabled_DISABLE {
		t.Fatalf("js_minification: got %s, want DISABLE", resolved.BuildPolicy.GetJsMinification())
	}
	if resolved.BuildPolicy.GetJsSourcemaps() != enabled.Enabled_DISABLE {
		t.Fatalf("js_sourcemaps: got %s, want DISABLE", resolved.BuildPolicy.GetJsSourcemaps())
	}
	if resolved.BuildPolicy.GetGoscriptCodeSplitting() != enabled.Enabled_ENABLE {
		t.Fatalf("goscript_code_splitting: got %s, want ENABLE", resolved.BuildPolicy.GetGoscriptCodeSplitting())
	}
	if len(resolved.PlatformIDs) == 0 {
		t.Fatal("expected platform ids from browser target")
	}
}

func TestResolveBuildTargetRejectsInvalidBuildPolicy(t *testing.T) {
	buildTarget := &bldr_project.BuildConfig{
		Targets:     []string{"browser"},
		BuildPolicy: manifest_build.NewBuildPolicy(enabled.Enabled(99), enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT),
	}

	if _, err := ResolveBuildTarget(buildTarget, nil, nil); err == nil {
		t.Fatal("expected invalid build policy error")
	}
}

func TestManifestBuilderBuildTargetStatusMetadata(t *testing.T) {
	ctrl := &Controller{
		manifestBuilderBuildTargets: make(map[string][]string),
	}
	mbc := NewManifestBuilderConfigWithTargetPlatforms(
		"spacewave-dist",
		"release",
		"desktop/darwin/arm64",
		"devtool",
		[]string{"desktop/darwin/arm64", "desktop/linux/amd64"},
	)

	ctrl.addManifestBuilderBuildTarget(mbc, "desktop")
	ctrl.addManifestBuilderBuildTarget(mbc, "desktop")
	ctrl.addManifestBuilderBuildTarget(mbc, "release")

	targets := ctrl.getManifestBuilderBuildTargets(mbc.MarshalB58())
	if len(targets) != 2 || targets[0] != "desktop" || targets[1] != "release" {
		t.Fatalf("unexpected build target metadata: %#v", targets)
	}

	tr := &manifestBuilderTracker{
		c:    ctrl,
		conf: mbc,
	}
	status := tr.newStatus(ManifestBuilderStatusStateQueued, "queued", "")
	if len(status.BuildTargetIDs) != 2 || status.BuildTargetIDs[0] != "desktop" || status.BuildTargetIDs[1] != "release" {
		t.Fatalf("unexpected build target ids: %#v", status.BuildTargetIDs)
	}
	if len(status.TargetPlatformIDs) != 2 || status.TargetPlatformIDs[0] != "desktop/darwin/arm64" || status.TargetPlatformIDs[1] != "desktop/linux/amd64" {
		t.Fatalf("unexpected target platform ids: %#v", status.TargetPlatformIDs)
	}
}

func TestManifestBuilderTrackerLifecycleStatusPreservesFiniteBuildMetadata(t *testing.T) {
	ctrl := &Controller{
		manifestBuilderBuildTargets: make(map[string][]string),
	}
	mbc := NewManifestBuilderConfigWithTargetPlatforms(
		"spacewave-dist",
		"release",
		"desktop/darwin/arm64",
		"devtool",
		[]string{"desktop/darwin/arm64"},
	)
	ctrl.addManifestBuilderBuildTarget(mbc, "desktop")

	sink := &recordingManifestBuilderStatusSink{}
	ctrl.manifestBuilderStatusSink = sink
	tr := &manifestBuilderTracker{
		c:    ctrl,
		conf: mbc,
	}
	tr.status = tr.newStatus(ManifestBuilderStatusStateQueued, "queued", "")

	tr.SetManifestBuilderLifecycleStatus(manifest_builder_controller.ManifestBuilderLifecycleStatus{
		State:    manifest_builder_controller.ManifestBuilderLifecycleStateDone,
		CacheHit: true,
		Summary:  "startup cache hit",
	})
	cacheHit := sink.last(t)
	if cacheHit.State != ManifestBuilderStatusStateDone || !cacheHit.CacheHit || cacheHit.Summary != "startup cache hit" {
		t.Fatalf("unexpected cache-hit status: %#v", cacheHit)
	}
	if len(cacheHit.BuildTargetIDs) != 1 || cacheHit.BuildTargetIDs[0] != "desktop" {
		t.Fatalf("cache-hit status lost finite build targets: %#v", cacheHit.BuildTargetIDs)
	}

	tr.SetManifestBuilderLifecycleStatus(manifest_builder_controller.ManifestBuilderLifecycleStatus{
		State:       manifest_builder_controller.ManifestBuilderLifecycleStateRunning,
		FullRebuild: true,
		Summary:     "full rebuild",
	})
	fullRebuild := sink.last(t)
	if fullRebuild.State != ManifestBuilderStatusStateRunning || !fullRebuild.FullRebuild || fullRebuild.HotRebuild {
		t.Fatalf("unexpected full rebuild status: %#v", fullRebuild)
	}

	tr.SetManifestBuilderLifecycleStatus(manifest_builder_controller.ManifestBuilderLifecycleStatus{
		State:                   manifest_builder_controller.ManifestBuilderLifecycleStateRunning,
		HotRebuild:              true,
		WatchedFileCount:        2,
		DependencyRebuildReason: "manifest dependency changed: web",
		Summary:                 "hot rebuild",
	})
	hotRebuild := sink.last(t)
	if hotRebuild.State != ManifestBuilderStatusStateRunning || !hotRebuild.HotRebuild || hotRebuild.FullRebuild {
		t.Fatalf("unexpected hot rebuild status: %#v", hotRebuild)
	}
	if hotRebuild.DependencyRebuildReason != "manifest dependency changed: web" || hotRebuild.WatchedFileCount != 2 {
		t.Fatalf("unexpected dependency rebuild metadata: %#v", hotRebuild)
	}
	if len(hotRebuild.TargetPlatformIDs) != 1 || hotRebuild.TargetPlatformIDs[0] != "desktop/darwin/arm64" {
		t.Fatalf("hot rebuild status lost target platform ids: %#v", hotRebuild.TargetPlatformIDs)
	}
}

type recordingManifestBuilderStatusSink struct {
	statuses []ManifestBuilderStatus
}

func (s *recordingManifestBuilderStatusSink) SetManifestBuilderStatus(status ManifestBuilderStatus) {
	s.statuses = append(s.statuses, status)
}

func (s *recordingManifestBuilderStatusSink) last(t *testing.T) ManifestBuilderStatus {
	t.Helper()
	if len(s.statuses) == 0 {
		t.Fatal("expected recorded status")
	}
	return s.statuses[len(s.statuses)-1]
}
