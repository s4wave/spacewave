package bldr_platform

import (
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/util/gotargets"
	"github.com/pkg/errors"
)

// PlatformID_NATIVE builds Go binaries in the native executable format.
// Builds a native binary with embedded assets (i.e. a .exe).
// Uses the NativePlatform base type to parse the platform ID.
const PlatformID_NATIVE = "native"

// NativePlatform is a base type for any go compiler based platform ID.
type NativePlatform struct {
	// GOOS is the go operating system type.
	// If empty, use the host go os.
	GOOS *string
	// GOARCH is the go architecture.
	// If empty, use the host go arch.
	GOARCH *string
	// GOARM is the Go arm version.
	// Only used if GOOS=linux and GOARCH=arm
	// If empty (zero), use v7.
	GOARM *int
	// InputPlatformID was the parsed platform ID string, if any.
	InputPlatformID string
}

// ToNativePlatform type-asserts the platform to NativePlatform.
//
// Returns nil if type doesn't match
func ToNativePlatform(p Platform) *NativePlatform {
	np, _ := p.(*NativePlatform)
	return np
}

// ParseNativePlatform parses a Go compiler based platform ID.
func ParseNativePlatform(str string) (*NativePlatform, error) {
	components := strings.Split(str, "/")
	if len(components) == 0 || components[0] != PlatformID_NATIVE {
		return nil, errors.Errorf("not a native platform id: %s", str)
	}
	goOsArches := gotargets.GetOsArchValues()
	pt := &NativePlatform{InputPlatformID: str}
	var arches []string
	for _, component := range components[1:] {
		if armVerStr, ok := strings.CutPrefix(component, "armv"); ok {
			armVer, err := strconv.Atoi(armVerStr)
			if err != nil || armVer < 5 || armVer > 8 {
				return nil, errors.Wrapf(err, "invalid arm version: %s", armVerStr)
			}
			var goarch string
			var goarm *int
			if armVer == 8 {
				goarch = "arm64"
				goarm = nil
			} else {
				goarch = "arm"
				goarm = &armVer
			}

			if pt.GOARCH == nil {
				pt.GOARCH = &goarch
			} else if *pt.GOARCH != goarch {
				return nil, errors.Errorf("conflicting values: %s and %s", *pt.GOARCH, goarch)
			}
			if pt.GOARM == nil {
				pt.GOARM = goarm
			} else if goarm == nil || *pt.GOARM != *goarm {
				return nil, errors.Errorf("conflicting values: %v and %v", *pt.GOARM, *goarm)
			}
			continue
		}
		// Handle special js/wasm and wasi/wasm combinations
		if component == "js" || component == "wasi" {
			goos := component
			if pt.GOOS != nil {
				return nil, errors.Errorf("multiple GOOS values: %s and %s", *pt.GOOS, goos)
			}
			pt.GOOS = &goos
			arches = []string{"wasm"}
			continue
		}
		if goosArches, isGoos := goOsArches[component]; isGoos {
			goos := component
			if pt.GOOS != nil {
				return nil, errors.Errorf("multiple GOOS values: %s and %s", *pt.GOOS, goos)
			}
			pt.GOOS = &goos
			arches = goosArches
			continue
		}
		if pt.GOOS != nil {
			if slices.Contains(arches, component) {
				if pt.GOARCH != nil {
					return nil, errors.Errorf("multiple GOARCH values: %s and %s", *pt.GOARCH, component)
				}
				goarch := component
				pt.GOARCH = &goarch
				continue
			}
		}
		// accept GOARM as /arm/6 or /arm/armv6
		if pt.GOARCH != nil && *pt.GOARCH == "arm" {
			goarm := strings.TrimPrefix(component, "armv")
			if val, err := strconv.Atoi(goarm); err == nil {
				if val < 5 || val > 7 {
					return nil, errors.Errorf("invalid GOARM version: %d", val)
				}
			}
		}
		return nil, errors.Errorf("unexpected platform id component: %s", component)
	}
	return pt, nil
}

// GetGOOS returns the GOOS if set or the host GOOs if not.
func (n *NativePlatform) GetGOOS() string {
	if n.GOOS != nil && *n.GOOS != "" {
		return *n.GOOS
	}
	return runtime.GOOS
}

// GetGOARCH returns the GOARCH if set or the host GOARCH if not.
func (n *NativePlatform) GetGOARCH() string {
	if n.GOARCH != nil && *n.GOARCH != "" {
		return *n.GOARCH
	}
	return runtime.GOARCH
}

// GetGOARM returns the GOARM to use or 0 if not applicable.
func (n *NativePlatform) GetGOARM() int {
	if n.GOARM != nil && *n.GOARM != 0 {
		return *n.GOARM
	}
	switch n.GetGOARCH() {
	case "arm64":
		return 0
	case "arm":
		return 7
	default:
		return 0
	}
}

// GetInputPlatformID returns the platform ID used when parsing.
// If unknown, return the output of GetPlatformID instead.
func (n *NativePlatform) GetInputPlatformID() string {
	if n.InputPlatformID != "" {
		return n.InputPlatformID
	}
	return n.GetPlatformID()
}

// GetPlatformID converts the platform into a fully qualified platform ID.
// There should be exactly one representation of the platform ID possible.
func (n *NativePlatform) GetPlatformID() string {
	// build the platform ID
	idParts := []string{
		PlatformID_NATIVE,
		n.GetGOOS(),
	}
	goArch := n.GetGOARCH()
	if goArch == "arm" {
		goArm := 7
		if n.GOARM != nil && *n.GOARM != 0 {
			goArm = *n.GOARM
		}
		idParts = append(idParts, "armv"+strconv.Itoa(goArm))
	} else {
		idParts = append(idParts, goArch)
	}

	return strings.Join(idParts, "/")
}

// GetBasePlatformID returns the base platform identifier w/o arch specifics.
// Values: PlatformID_NATIVE and PlatformID_JS
func (n *NativePlatform) GetBasePlatformID() string {
	return PlatformID_NATIVE
}

// GetExecutableExt returns the extension used for executables.
func (n *NativePlatform) GetExecutableExt() string {
	goos := n.GetGOOS()
	goarch := n.GetGOARCH()

	if goos == "windows" {
		return ".exe"
	}
	if goos == "js" {
		return ".mjs"
	}
	if goarch == "wasm" || goarch == "wasm32" || goarch == "wasm64" {
		return ".wasm"
	}

	return ""
}

// _ is a type assertion
var _ Platform = (*NativePlatform)(nil)
