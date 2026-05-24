package bldr_manifest_build

import (
	"testing"

	"github.com/aperturerobotics/util/enabled"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

func TestParseEnabled(t *testing.T) {
	tests := []struct {
		raw  string
		want enabled.Enabled
	}{
		{"", enabled.Enabled_DEFAULT},
		{"default", enabled.Enabled_DEFAULT},
		{"ENABLE", enabled.Enabled_ENABLE},
		{" disable ", enabled.Enabled_DISABLE},
	}
	for _, test := range tests {
		got, err := ParseEnabled(test.raw)
		if err != nil {
			t.Fatalf("ParseEnabled(%q): %v", test.raw, err)
		}
		if got != test.want {
			t.Fatalf("ParseEnabled(%q): got %s, want %s", test.raw, got, test.want)
		}
	}
}

func TestParseEnabledRejectsInvalidValue(t *testing.T) {
	_, err := ParseEnabled("readable")
	if err == nil {
		t.Fatal("expected invalid value error")
	}
}

func TestBuildPolicyValidateRejectsInvalidEnum(t *testing.T) {
	policy := NewBuildPolicy(enabled.Enabled(99), enabled.Enabled_DEFAULT)
	if err := policy.Validate(); err == nil {
		t.Fatal("expected invalid enum error")
	}
}

func TestBuildPolicyMerge(t *testing.T) {
	base := NewBuildPolicy(enabled.Enabled_ENABLE, enabled.Enabled_DISABLE)
	override := NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_ENABLE)

	got := base.Merge(override)
	if got.GetJsMinification() != enabled.Enabled_ENABLE {
		t.Fatalf("js_minification: got %s, want ENABLE", got.GetJsMinification())
	}
	if got.GetJsSourcemaps() != enabled.Enabled_ENABLE {
		t.Fatalf("js_sourcemaps: got %s, want ENABLE", got.GetJsSourcemaps())
	}
}

func TestBuildPolicyResolveDefaultsByBuildType(t *testing.T) {
	policy := NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT)

	if !policy.ResolveJsMinification(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("release DEFAULT should minify JavaScript")
	}
	if policy.ResolveJsMinification(bldr_manifest.BuildType_DEV) {
		t.Fatal("dev DEFAULT should not minify JavaScript")
	}
	if policy.ResolveJsSourcemaps(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("release DEFAULT should not emit sourcemaps")
	}
	if !policy.ResolveJsSourcemaps(bldr_manifest.BuildType_DEV) {
		t.Fatal("dev DEFAULT should emit sourcemaps")
	}
}

func TestBuildPolicyResolveExplicitValues(t *testing.T) {
	policy := NewBuildPolicy(enabled.Enabled_DISABLE, enabled.Enabled_ENABLE)

	if policy.ResolveJsMinification(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("explicit DISABLE should keep release JavaScript readable")
	}
	if !policy.ResolveJsSourcemaps(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("explicit ENABLE should emit release sourcemaps")
	}
}
