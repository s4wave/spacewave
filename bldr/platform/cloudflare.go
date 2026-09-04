package bldr_platform

import (
	"strings"

	"github.com/pkg/errors"
)

// PlatformID_CLOUDFLARE identifies the Cloudflare Workers platform.
const PlatformID_CLOUDFLARE = "cloudflare-workers"

// CloudflarePlatform is the Cloudflare Workers platform type.
type CloudflarePlatform struct {
	// InputPlatformID was the parsed platform ID string, if any.
	InputPlatformID string
}

// NewCloudflarePlatform constructs a new default CloudflarePlatform.
func NewCloudflarePlatform() *CloudflarePlatform {
	return &CloudflarePlatform{
		InputPlatformID: PlatformID_CLOUDFLARE,
	}
}

// ParseCloudflarePlatform parses the Cloudflare Workers platform ID.
func ParseCloudflarePlatform(str string) (*CloudflarePlatform, error) {
	components := strings.Split(str, "/")

	if len(components) == 0 || components[0] != PlatformID_CLOUDFLARE {
		return nil, errors.Errorf("not a cloudflare-workers platform id: %s", str)
	}

	if len(components) > 1 {
		return nil, errors.Errorf("unrecognized portion of cloudflare-workers platform id: %s", strings.Join(components[1:], "/"))
	}

	return &CloudflarePlatform{InputPlatformID: str}, nil
}

// GetInputPlatformID returns the platform ID used when parsing.
// If unknown, return the output of GetPlatformID instead.
func (n *CloudflarePlatform) GetInputPlatformID() string {
	if n.InputPlatformID != "" {
		return n.InputPlatformID
	}
	return n.GetPlatformID()
}

// GetPlatformID converts the platform into a fully qualified platform ID.
// There should be exactly one representation of the platform ID possible.
func (n *CloudflarePlatform) GetPlatformID() string {
	return PlatformID_CLOUDFLARE
}

// GetBasePlatformID returns the base platform identifier w/o arch specifics.
// Values: PlatformID_DESKTOP, PlatformID_JS, PlatformID_CLOUDFLARE, and
// PlatformID_NONE.
func (n *CloudflarePlatform) GetBasePlatformID() string {
	return PlatformID_CLOUDFLARE
}

// GetExecutableExt returns the extension used for the primary executable artifact.
func (n *CloudflarePlatform) GetExecutableExt() string {
	return ".mjs"
}

// _ is a type assertion
var _ Platform = (*CloudflarePlatform)(nil)
