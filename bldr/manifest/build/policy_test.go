package bldr_manifest_build

import (
	"strings"
	"testing"

	"github.com/aperturerobotics/util/enabled"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// TestParseEnabled checks parsing of each enabled value, ignoring case and spaces.
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

// TestParseEnabledRejectsInvalidValue checks that an unknown value fails.
func TestParseEnabledRejectsInvalidValue(t *testing.T) {
	// Parse a value outside the enum.
	_, err := ParseEnabled("readable")
	if err == nil {
		t.Fatal("expected invalid value error")
	}
}

// TestBuildPolicyValidateRejectsInvalidEnum checks that Validate names the
// field holding an out-of-range value.
func TestBuildPolicyValidateRejectsInvalidEnum(t *testing.T) {
	tests := []struct {
		name        string
		policy      *BuildPolicy
		wantInError string
	}{
		{
			name:        "js minification",
			policy:      NewBuildPolicy(enabled.Enabled(99), enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT),
			wantInError: "js_minification",
		},
		{
			name:        "js sourcemaps",
			policy:      NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled(99), enabled.Enabled_DEFAULT),
			wantInError: "js_sourcemaps",
		},
		{
			name:        "goscript code splitting",
			policy:      NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT, enabled.Enabled(99)),
			wantInError: "goscript_code_splitting",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.Validate()
			if err == nil {
				t.Fatal("expected invalid enum error")
			}
			if !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("error = %q, want field %q", err, test.wantInError)
			}
		})
	}
}

// TestBuildPolicyMerge checks that an override replaces each field unless it
// is DEFAULT.
func TestBuildPolicyMerge(t *testing.T) {
	// Merge a policy with an override that sets JS minification to DEFAULT.
	base := NewBuildPolicy(enabled.Enabled_ENABLE, enabled.Enabled_DISABLE, enabled.Enabled_DISABLE)
	override := NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_ENABLE, enabled.Enabled_ENABLE)

	// The DEFAULT field keeps its base value and the explicit fields replace theirs.
	got := base.Merge(override)
	if got.GetJsMinification() != enabled.Enabled_ENABLE {
		t.Fatalf("js_minification: got %s, want ENABLE", got.GetJsMinification())
	}
	if got.GetJsSourcemaps() != enabled.Enabled_ENABLE {
		t.Fatalf("js_sourcemaps: got %s, want ENABLE", got.GetJsSourcemaps())
	}
	if got.GetGoscriptCodeSplitting() != enabled.Enabled_ENABLE {
		t.Fatalf("goscript_code_splitting: got %s, want ENABLE", got.GetGoscriptCodeSplitting())
	}

	// An explicit DISABLE replaces ENABLE.
	disabled := NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT, enabled.Enabled_ENABLE).Merge(
		NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT, enabled.Enabled_DISABLE),
	)
	if disabled.GetGoscriptCodeSplitting() != enabled.Enabled_DISABLE {
		t.Fatalf("goscript_code_splitting: got %s, want DISABLE", disabled.GetGoscriptCodeSplitting())
	}

	// A DEFAULT override keeps the existing value.
	defaulted := NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT, enabled.Enabled_DISABLE).Merge(
		NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT),
	)
	if defaulted.GetGoscriptCodeSplitting() != enabled.Enabled_DISABLE {
		t.Fatalf("goscript_code_splitting: got %s, want existing DISABLE when override is DEFAULT", defaulted.GetGoscriptCodeSplitting())
	}
}

// TestBuildPolicyResolveDefaultsByBuildType checks the DEFAULT resolution for
// release and dev builds.
func TestBuildPolicyResolveDefaultsByBuildType(t *testing.T) {
	// Release minifies without sourcemaps, dev emits sourcemaps unminified.
	policy := NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT)
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

	// Both build types split GoScript bundles.
	if !policy.ResolveGoScriptCodeSplitting(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("release DEFAULT should split GoScript bundles")
	}
	if !policy.ResolveGoScriptCodeSplitting(bldr_manifest.BuildType_DEV) {
		t.Fatal("dev DEFAULT should split GoScript bundles")
	}
}

// TestBuildPolicyResolveExplicitValues checks that explicit values override
// the build type defaults.
func TestBuildPolicyResolveExplicitValues(t *testing.T) {
	// Explicit values apply to a release build.
	policy := NewBuildPolicy(enabled.Enabled_DISABLE, enabled.Enabled_ENABLE, enabled.Enabled_ENABLE)
	if policy.ResolveJsMinification(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("explicit DISABLE should keep release JavaScript readable")
	}
	if !policy.ResolveJsSourcemaps(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("explicit ENABLE should emit release sourcemaps")
	}
	if !policy.ResolveGoScriptCodeSplitting(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("explicit ENABLE should split release GoScript bundles")
	}

	// An explicit DISABLE keeps both build types single-file.
	disabled := NewBuildPolicy(enabled.Enabled_DEFAULT, enabled.Enabled_DEFAULT, enabled.Enabled_DISABLE)
	if disabled.ResolveGoScriptCodeSplitting(bldr_manifest.BuildType_RELEASE) {
		t.Fatal("explicit DISABLE should keep release GoScript bundles single-file")
	}
	if disabled.ResolveGoScriptCodeSplitting(bldr_manifest.BuildType_DEV) {
		t.Fatal("explicit DISABLE should keep dev GoScript bundles single-file")
	}
}
