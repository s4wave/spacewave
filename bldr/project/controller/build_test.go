//go:build !js

package bldr_project_controller

import (
	"bytes"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
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

	var gotOverride *configset_proto.ControllerConfig
	err := ForManifestSelector(
		[]string{"spacewave-dist", "spacewave-launcher"},
		platformIDs,
		func(manifestID, platformID string) (bool, error) {
			mbc := NewManifestBuilderConfigWithTargetPlatforms(
				manifestID,
				"release",
				platformID,
				"devtool",
				platformIDs,
			)
			if o := manifestOverrides[manifestID]; o != nil {
				mbc.BuilderConfigOverride = o.CloneVT()
			}
			if manifestID == "spacewave-dist" {
				gotOverride = mbc.GetBuilderConfigOverride()
			} else if mbc.GetBuilderConfigOverride() != nil {
				t.Fatalf("unexpected override for manifest %s", manifestID)
			}
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
}
