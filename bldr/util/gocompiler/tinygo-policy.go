package gocompiler

import (
	"github.com/aperturerobotics/util/enabled"
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	bldr_platform_go "github.com/s4wave/spacewave/bldr/platform/go"
)

// ResolveTinyGoEnabled resolves TinyGo policy and validates supported platforms.
func ResolveTinyGoEnabled(
	buildPlatform bldr_platform.Platform,
	enableTinygoOpt enabled.Enabled,
	defaultEnabled bool,
) (bool, error) {
	useTinygo := enableTinygoOpt.IsEnabled(defaultEnabled)
	if !useTinygo {
		return false, nil
	}
	if _, err := bldr_platform_go.PlatformToTinyGoTarget(buildPlatform); err != nil {
		return false, errors.Wrap(err, "tinygo enabled")
	}
	return true, nil
}
