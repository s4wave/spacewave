package bldr_platform_go

import (
	"strconv"

	bldr_platform "github.com/aperturerobotics/bldr/platform"
	"github.com/pkg/errors"
)

// PlatformToGoEnv builds the Go environment variables for the desired platform.
func PlatformToGoEnv(plat bldr_platform.Platform) ([]string, error) {
	var vars []string
	switch p := plat.(type) {
	case *bldr_platform.NativePlatform:
		vars = append(vars, "GOOS="+p.GetGOOS())
		vars = append(vars, "GOARCH="+p.GetGOARCH())
		if goArm := p.GetGOARM(); goArm != 0 {
			vars = append(vars, "GOARM="+strconv.Itoa(goArm))
		}
	case *bldr_platform.WebPlatform:
		vars = append(vars, "GOOS=js", "GOARCH=wasm")
	default:
		return nil, errors.Errorf("unrecognized go-compiler platform: %s", plat.GetPlatformID())
	}
	return vars, nil
}

// PlatformToTinyGoTarget converts the Go platform into a tinygo platform.
func PlatformToTinyGoTarget(plat bldr_platform.Platform) (string, error) {
	switch plat.(type) {
	case *bldr_platform.WebPlatform:
		return "wasm-unknown", nil
	default:
		return "", errors.Errorf("go-compiler platform does not support tinygo: %s", plat.GetPlatformID())
	}
}
