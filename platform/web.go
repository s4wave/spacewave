package platform

import (
	"strings"

	"github.com/pkg/errors"
)

// PlatformID_WEB builds Go binaries for the Web platform (WebAssembly).
const PlatformID_WEB = "web"

// WebPlatform represents the web platform type.
type WebPlatform struct {
	// PlatformID was the parsed platform ID string, if any.
	PlatformID string
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
	return &WebPlatform{PlatformID: str}, nil
}

// GetPlatformID converts the platform into a platform ID.
// Returns the original platform ID used to parse the platform, if possible.
func (n *WebPlatform) GetPlatformID() string {
	if n.PlatformID != "" {
		return n.PlatformID
	}

	return PlatformID_WEB
}

// _ is a type assertion
var _ Platform = (*WebPlatform)(nil)
