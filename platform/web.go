package bldr_platform

import (
	"strings"

	"github.com/pkg/errors"
)

// PlatformID_WEB builds Go binaries for the Web platform (WebAssembly).
const PlatformID_WEB = "web"

// WebPlatform represents the web platform type.
type WebPlatform struct {
	// InputPlatformID was the parsed platform ID string, if any.
	InputPlatformID string
}

// NewWebPlatform constructs a new default WebPlatform.
func NewWebPlatform() *WebPlatform {
	return &WebPlatform{InputPlatformID: PlatformID_WEB}
}

// ParseWebPlatform parses web platform based platform ID.
func ParseWebPlatform(str string) (*WebPlatform, error) {
	components := strings.Split(str, "/")
	if len(components) == 0 || components[0] != PlatformID_WEB {
		return nil, errors.Errorf("not a web platform id: %s", str)
	}
	if len(components) > 1 {
		return nil, errors.Errorf("unrecognized portion of web platform id: %s", strings.Join(components[1:], "/"))
	}
	return &WebPlatform{InputPlatformID: str}, nil
}

// GetInputPlatformID returns the platform ID used when parsing.
// If unknown, return the output of GetPlatformID instead.
func (n *WebPlatform) GetInputPlatformID() string {
	if n.InputPlatformID != "" {
		return n.InputPlatformID
	}
	return n.GetPlatformID()
}

// GetPlatformID converts the platform into a fully qualified platform ID.
// There should be exactly one representation of the platform ID possible.
func (n *WebPlatform) GetPlatformID() string {
	return PlatformID_WEB
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
	return ".mjs"
}

// _ is a type assertion
var _ Platform = (*WebPlatform)(nil)
