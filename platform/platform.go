package platform

import (
	"strings"

	"github.com/pkg/errors"
)

// Platform is the common interface for platform types.
type Platform interface {
	// GetPlatformID converts the platform into a platform ID.
	// Returns the original platform ID used to parse the platform, if possible.
	GetPlatformID() string
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
