package bldr_platform_npm

import (
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"github.com/pkg/errors"
)

// Npm contains information passed to npm about the target platform.
type Npm struct {
	// Platform is the ID passed to --platform.
	// Examples: win32, darwin, linux
	// https://nodejs.org/api/process.html#processplatform
	Platform string
	// Arch is the ID passed to --arch.
	// Examples: x64, arm, arm64, ia32
	// https://nodejs.org/api/process.html#processarch
	Arch string
}

// ToNpmFlags converts the Npm object to npm flags.
func (n *Npm) ToNpmFlags() []string {
	flags := make([]string, 0, 2)
	if len(n.Platform) != 0 {
		flags = append(flags, "--platform="+n.Platform)
	}
	if len(n.Arch) != 0 {
		flags = append(flags, "--arch="+n.Arch)
	}
	return flags
}

// PlatformToNpm builds the Npm platform information for the desired target platform.
func PlatformToNpm(plat bldr_platform.Platform) (*Npm, error) {
	switch p := plat.(type) {
	case *bldr_platform.NativePlatform:
		return NativePlatformToNpm(p)
	default:
		return nil, errors.Errorf("unsupported platform for npm install: %s", plat.GetPlatformID())
	}
}

// NativePlatformToNpm builds the Npm platform information for the desired target platform.
func NativePlatformToNpm(native *bldr_platform.NativePlatform) (*Npm, error) {
	return &Npm{
		Platform: NpmPlatformForGoos(native.GetGOOS()),
		Arch:     NpmArchForGoArch(native.GetGOARCH()),
	}, nil
}

// GoOsToNpmPlatform is a mapping between GOOS and Npm platform values.
//
// If a value is not present in this list, assume the platform ID is the Goos value.
var GoOsToNpmPlatform = map[string]string{
	"sunos": "solaris",
	"ios":   "darwin",
}

// NpmPlatformForGoos returns the npm platform id for the given GOOS value.
//
// If unknown returns the goos value directly (assuming this works).
func NpmPlatformForGoos(goos string) string {
	plat := GoOsToNpmPlatform[goos]
	if plat == "" {
		return goos
	}
	return plat
}

// GoArchToNpmPlatform is a mapping between GOARCH and Npm arch values.
//
// If a value is not present in this list, assume the arch ID is the Goarch value.
var GoArchToNpmPlatform = map[string]string{
	"amd64":  "x64",
	"386":    "ia32",
	"mipsle": "mipsel",
}

// NpmArchForGoArch returns the npm arch id for the given GOARCH value.
//
// If unknown returns the goarch value directly (assuming this works).
func NpmArchForGoArch(goarch string) string {
	plat := GoArchToNpmPlatform[goarch]
	if plat == "" {
		return goarch
	}
	return plat
}
