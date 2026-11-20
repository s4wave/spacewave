package s4wave_frametrack

import (
	"strings"

	"github.com/pkg/errors"
)

// ValidateTarget requires a title and the keys identifying the served track.
func ValidateTarget(name, deviceObjectKey, capabilityID string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("frame object name is required")
	case strings.TrimSpace(deviceObjectKey) == "":
		return errors.New("frame object Device key is required")
	case strings.TrimSpace(capabilityID) == "":
		return errors.New("frame object capability ID is required")
	default:
		return nil
	}
}
