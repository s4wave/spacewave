package bldr_platform

import "github.com/pkg/errors"

// PlatformID_NONE indicates the manifest holds static files only.
const PlatformID_NONE = "none"

// NonePlatform represents a platform with static files only.
type NonePlatform struct {
	// InputPlatformID was the parsed platform ID string, if any.
	InputPlatformID string
}

// NewNonePlatform constructs a new NonePlatform.
func NewNonePlatform() *NonePlatform {
	return &NonePlatform{InputPlatformID: PlatformID_NONE}
}

// ParseNonePlatform parses a none platform ID.
func ParseNonePlatform(str string) (*NonePlatform, error) {
	if str != PlatformID_NONE {
		return nil, errors.Errorf("not a none platform id: %s", str)
	}
	return &NonePlatform{InputPlatformID: str}, nil
}

// GetInputPlatformID returns the platform ID used when parsing.
// If unknown, return the output of GetPlatformID instead.
func (n *NonePlatform) GetInputPlatformID() string {
	if n.InputPlatformID != "" {
		return n.InputPlatformID
	}
	return n.GetPlatformID()
}

// GetPlatformID converts the platform into a fully qualified platform ID.
// There should be exactly one representation of the platform ID possible.
func (n *NonePlatform) GetPlatformID() string {
	return PlatformID_NONE
}

// GetBasePlatformID returns the base platform identifier w/o arch specifics.
// Values: PlatformID_DESKTOP, PlatformID_JS, and PlatformID_NONE
func (n *NonePlatform) GetBasePlatformID() string {
	return PlatformID_NONE
}

// GetExecutableExt returns the extension used for executables.
func (n *NonePlatform) GetExecutableExt() string {
	return ""
}

// _ is a type assertion
var _ Platform = (*NonePlatform)(nil)
