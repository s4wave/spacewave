package bldr_platform

import (
	"strings"

	"github.com/pkg/errors"
)

// PlatformID_JS builds binaries for JavaScript environments (quickjs or WebWorker).
const PlatformID_JS = "js"

// JsPlatform represents the JavaScript platform type.
type JsPlatform struct {
	// InputPlatformID was the parsed platform ID string, if any.
	InputPlatformID string
}

// NewJsPlatform constructs a new default JsPlatform.
func NewJsPlatform() *JsPlatform {
	return &JsPlatform{
		InputPlatformID: PlatformID_JS,
	}
}

// ParseJsPlatform parses JavaScript platform based platform ID.
func ParseJsPlatform(str string) (*JsPlatform, error) {
	components := strings.Split(str, "/")

	if len(components) == 0 || components[0] != PlatformID_JS {
		return nil, errors.Errorf("not a js platform id: %s", str)
	}

	if len(components) > 1 {
		return nil, errors.Errorf("unrecognized portion of js platform id: %s", strings.Join(components[1:], "/"))
	}

	return &JsPlatform{InputPlatformID: str}, nil
}

// GetInputPlatformID returns the platform ID used when parsing.
// If unknown, return the output of GetPlatformID instead.
func (n *JsPlatform) GetInputPlatformID() string {
	if n.InputPlatformID != "" {
		return n.InputPlatformID
	}
	return n.GetPlatformID()
}

// GetPlatformID converts the platform into a fully qualified platform ID.
// There should be exactly one representation of the platform ID possible.
func (n *JsPlatform) GetPlatformID() string {
	return PlatformID_JS
}

// GetBasePlatformID returns the base platform identifier w/o arch specifics.
// Values: PlatformID_NATIVE, PlatformID_JS, and PlatformID_NONE
func (n *JsPlatform) GetBasePlatformID() string {
	return PlatformID_JS
}

// GetExecutableExt returns the extension used for the primary executable artifact.
func (n *JsPlatform) GetExecutableExt() string {
	return ".mjs"
}

// _ is a type assertion
var _ Platform = (*JsPlatform)(nil)
