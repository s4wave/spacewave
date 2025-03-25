package bldr_platform

import (
	"strings"

	"github.com/pkg/errors"
)

// PlatformID_WEB builds Go binaries for the Web platform (WebAssembly).
const PlatformID_WEB = "web"

// WebPlatform represents the web platform type.
type WebPlatform struct {
	// WEBARCH is the web architecture type.
	// Values: "js" (default) or "wasip2"
	WEBARCH *string
	// InputPlatformID was the parsed platform ID string, if any.
	InputPlatformID string
}

// NewWebPlatform constructs a new default WebPlatform.
func NewWebPlatform() *WebPlatform {
	js := "js"
	return &WebPlatform{
		WEBARCH:         &js,
		InputPlatformID: PlatformID_WEB,
	}
}

// ParseWebPlatform parses web platform based platform ID.
func ParseWebPlatform(str string) (*WebPlatform, error) {
	components := strings.Split(str, "/")
	if len(components) == 0 || components[0] != PlatformID_WEB {
		return nil, errors.Errorf("not a web platform id: %s", str)
	}

	js := "js"
	platform := &WebPlatform{
		WEBARCH:         &js,
		InputPlatformID: str,
	}

	if len(components) > 2 {
		return nil, errors.Errorf("unrecognized portion of web platform id: %s", strings.Join(components[2:], "/"))
	}

	if len(components) == 2 {
		arch := components[1]
		if arch != "js" && arch != "wasip2" {
			return nil, errors.Errorf("invalid web architecture: %s", arch)
		}
		platform.WEBARCH = &arch
	}

	return platform, nil
}

// GetInputPlatformID returns the platform ID used when parsing.
// If unknown, return the output of GetPlatformID instead.
func (n *WebPlatform) GetInputPlatformID() string {
	if n.InputPlatformID != "" {
		return n.InputPlatformID
	}
	return n.GetPlatformID()
}

// GetWEBARCH returns the WEBARCH if set or "js" if not.
func (n *WebPlatform) GetWEBARCH() string {
	if n.WEBARCH != nil && *n.WEBARCH != "" {
		return *n.WEBARCH
	}
	return "js"
}

// GetPlatformID converts the platform into a fully qualified platform ID.
// There should be exactly one representation of the platform ID possible.
func (n *WebPlatform) GetPlatformID() string {
	if n.GetWEBARCH() == "js" {
		return PlatformID_WEB
	}
	return PlatformID_WEB + "/" + n.GetWEBARCH()
}

// GetBasePlatformID returns the base platform identifier w/o arch specifics.
// Values: PlatformID_NATIVE and PlatformID_WEB
func (n *WebPlatform) GetBasePlatformID() string {
	return PlatformID_WEB
}

// GetExecutableExt returns the extension used for executables.
func (n *WebPlatform) GetExecutableExt() string {
	return ".wasm"
}

// GetEntrypointExt returns the extension used for the entrypoint. May be empty.
// if empty, it is assumed that there is no alterative entrypoint file.
func (n *WebPlatform) GetEntrypointExt() string {
	if n.GetWEBARCH() == "js" {
		return ".mjs"
	}

	return ""
}

// _ is a type assertion
var _ Platform = (*WebPlatform)(nil)
