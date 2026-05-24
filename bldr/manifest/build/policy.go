package bldr_manifest_build

import (
	"strings"

	"github.com/aperturerobotics/util/enabled"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// ParseEnabled parses a build policy option value.
func ParseEnabled(raw string) (enabled.Enabled, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default":
		return enabled.Enabled_DEFAULT, nil
	case "enable":
		return enabled.Enabled_ENABLE, nil
	case "disable":
		return enabled.Enabled_DISABLE, nil
	default:
		return enabled.Enabled_DEFAULT, errors.Errorf("invalid build policy value %q: expected default, enable, or disable", raw)
	}
}

// FormatEnabled formats a build policy option value for flags and env vars.
func FormatEnabled(value enabled.Enabled) string {
	switch value {
	case enabled.Enabled_ENABLE:
		return "enable"
	case enabled.Enabled_DISABLE:
		return "disable"
	default:
		return "default"
	}
}

// NewBuildPolicy constructs a BuildPolicy from JavaScript policy options.
func NewBuildPolicy(jsMinification, jsSourcemaps enabled.Enabled) *BuildPolicy {
	return &BuildPolicy{
		JsMinification: jsMinification,
		JsSourcemaps:   jsSourcemaps,
	}
}

// Validate validates the BuildPolicy.
func (p *BuildPolicy) Validate() error {
	if p == nil {
		return nil
	}
	if err := p.GetJsMinification().Validate(); err != nil {
		return errors.Wrap(err, "js_minification")
	}
	if err := p.GetJsSourcemaps().Validate(); err != nil {
		return errors.Wrap(err, "js_sourcemaps")
	}
	return nil
}

// Merge merges override into p using enabled.Enabled DEFAULT as "not set".
func (p *BuildPolicy) Merge(override *BuildPolicy) *BuildPolicy {
	merged := &BuildPolicy{}
	if p != nil {
		merged = p.CloneVT()
	}
	if override == nil {
		return merged
	}
	merged.JsMinification = merged.GetJsMinification().Merge(override.GetJsMinification())
	merged.JsSourcemaps = merged.GetJsSourcemaps().Merge(override.GetJsSourcemaps())
	return merged
}

// ResolveJsMinification resolves JavaScript minification against BuildType defaults.
func (p *BuildPolicy) ResolveJsMinification(buildType bldr_manifest.BuildType) bool {
	return p.GetJsMinification().IsEnabled(buildType.IsRelease())
}

// ResolveJsSourcemaps resolves JavaScript sourcemaps against BuildType defaults.
func (p *BuildPolicy) ResolveJsSourcemaps(buildType bldr_manifest.BuildType) bool {
	return p.GetJsSourcemaps().IsEnabled(buildType.IsDev())
}
