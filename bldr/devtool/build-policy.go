//go:build !js

package devtool

import (
	"github.com/pkg/errors"
	manifest_build "github.com/s4wave/spacewave/bldr/manifest/build"
)

// BuildPolicyOverride parses the build policy flags into a command override.
func (a *DevtoolArgs) BuildPolicyOverride() (*manifest_build.BuildPolicy, error) {
	jsMinification, err := manifest_build.ParseEnabled(a.JSMinification)
	if err != nil {
		return nil, errors.Wrap(err, "js-minification")
	}
	jsSourcemaps, err := manifest_build.ParseEnabled(a.JSSourcemaps)
	if err != nil {
		return nil, errors.Wrap(err, "js-sourcemaps")
	}
	return manifest_build.NewBuildPolicy(jsMinification, jsSourcemaps), nil
}
