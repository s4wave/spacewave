package bldr_platform_go

import (
	"strconv"

	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
)

var (
	// ErrUnsupportedPlatform indicates the platform is not supported by the Go compiler.
	ErrUnsupportedPlatform = errors.New("unsupported go-compiler platform")
	// ErrTinyGoUnsupported indicates the platform does not support TinyGo.
	ErrTinyGoUnsupported = errors.New("go-compiler platform does not support tinygo")
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
	case *bldr_platform.JsPlatform:
		vars = append(vars, "GOOS=js", "GOARCH=wasm")
	default:
		return nil, errors.Wrapf(ErrUnsupportedPlatform, "platform: %s", plat.GetPlatformID())
	}
	return vars, nil
}

// PlatformToTinyGoTarget converts the Go platform into a tinygo platform.
func PlatformToTinyGoTarget(plat bldr_platform.Platform) (string, error) {
	switch p := plat.(type) {
	case *bldr_platform.NativePlatform:
		if p.GetGOOS() == "wasi" && p.GetGOARCH() == "wasm" {
			return "wasm-unknown", nil
		}
		return "", errors.Wrapf(ErrTinyGoUnsupported, "platform: %s", plat.GetPlatformID())
	default:
		return "", errors.Wrapf(ErrTinyGoUnsupported, "platform: %s", plat.GetPlatformID())
	}
}
