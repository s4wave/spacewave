package bldr_platform

import (
	"strings"

	"github.com/pkg/errors"
)

// Platform is the common interface for platform types.
type Platform interface {
	// GetInputPlatformID returns the platform ID used when parsing.
	// If unknown, return the output of GetPlatformID instead.
	GetInputPlatformID() string
	// GetPlatformID converts the platform into a fully qualified platform ID.
	// There should be exactly one representation of the platform ID possible.
	GetPlatformID() string
	// GetBasePlatformID returns the base platform identifier w/o arch specifics.
	// Values: PlatformID_NATIVE and PlatformID_WEB
	GetBasePlatformID() string
	// GetExecutableExt returns the extension used for executables. May be empty.
	GetExecutableExt() string
	// GetEntrypointExt returns the extension used for the entrypoint. May be empty.
	// if empty, it is assumed that there is no alterative entrypoint file.
	GetEntrypointExt() string
}

// ParsePlatform parses the given platform ID.
// Result can be either *NativePlatform or *WebPlatform.
// Returns nil, err if not recognized.
func ParsePlatform(id string) (Platform, error) {
	firstCmp, _, _ := strings.Cut(id, "/")
	switch firstCmp {
	case PlatformID_NATIVE:
		return ParseNativePlatform(id)
	case PlatformID_WEB:
		return ParseWebPlatform(id)
	default:
		return nil, errors.Errorf("unknown platform id: %s", id)
	}
}
