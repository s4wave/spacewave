package bldr_platform

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

// Target represents a deployment target with prioritized platform support.
type Target struct {
	// ID is the target identifier (e.g., "browser", "desktop").
	ID string
	// PlatformIDs is the ordered list of platform IDs (highest priority first).
	PlatformIDs []string
	// Description describes the target.
	Description string
}

// GetID returns the target identifier.
func (t *Target) GetID() string {
	if t == nil {
		return ""
	}
	return t.ID
}

// GetPlatformIDs returns the ordered list of platform IDs.
func (t *Target) GetPlatformIDs() []string {
	if t == nil {
		return nil
	}
	return t.PlatformIDs
}

// GetDescription returns the target description.
func (t *Target) GetDescription() string {
	if t == nil {
		return ""
	}
	return t.Description
}

// TargetID_Browser is the target ID for web browser environments.
const TargetID_Browser = "browser"

// TargetID_Desktop is the target ID for native desktop applications.
const TargetID_Desktop = "desktop"

// GetHostPlatformID returns the platform ID for the current host.
func GetHostPlatformID() string {
	return fmt.Sprintf("desktop/%s/%s", runtime.GOOS, runtime.GOARCH)
}

// BuiltinTargets contains the predefined targets.
// Note: desktop target uses the host platform, computed at call time via GetBuiltinTarget.
var BuiltinTargets = map[string]*Target{
	TargetID_Browser: {
		ID:          TargetID_Browser,
		PlatformIDs: []string{"web/js/wasm", PlatformID_JS},
		Description: "Web browser environment (WebAssembly + JavaScript)",
	},
}

// GetBuiltinTarget returns a builtin target by ID.
// For targets that depend on the host platform (like "desktop"), this computes the correct platform IDs.
func GetBuiltinTarget(id string) *Target {
	if target, ok := BuiltinTargets[id]; ok {
		return target
	}
	if id == TargetID_Desktop {
		return &Target{
			ID:          TargetID_Desktop,
			PlatformIDs: []string{GetHostPlatformID(), PlatformID_JS},
			Description: "Native desktop application with JavaScript fallback",
		}
	}
	return nil
}

// ParseTarget parses a target string, supporting built-in and parameterized targets.
// Examples: "browser", "desktop", "desktop/darwin/arm64"
func ParseTarget(id string) (*Target, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("target ID cannot be empty")
	}

	// Check built-in targets first
	if target := GetBuiltinTarget(id); target != nil {
		return target, nil
	}

	// Handle parameterized targets like "desktop/darwin/arm64"
	if after, ok := strings.CutPrefix(id, TargetID_Desktop+"/"); ok {
		suffix := after
		if suffix == "cross" {
			return &Target{
				ID:          id,
				PlatformIDs: GetAllNativePlatformIDs(),
				Description: "Cross-compile for all architectures",
			}, nil
		}

		// Parse as specific OS/arch
		platformID := "desktop/" + suffix
		if _, err := ParsePlatform(platformID); err != nil {
			return nil, errors.Wrapf(err, "invalid target platform: %s", suffix)
		}
		return &Target{
			ID:          id,
			PlatformIDs: []string{platformID, PlatformID_JS},
			Description: fmt.Sprintf("Desktop for %s", suffix),
		}, nil
	}

	return nil, errors.Errorf("unknown target: %s", id)
}

// GetAllNativePlatformIDs returns platform IDs for all common native targets.
func GetAllNativePlatformIDs() []string {
	return []string{
		"desktop/darwin/amd64",
		"desktop/darwin/arm64",
		"desktop/linux/amd64",
		"desktop/linux/arm64",
		"desktop/windows/amd64",
		"desktop/windows/arm64",
		PlatformID_JS,
	}
}

// SelectPlatformForCompiler selects the best platform from the target for a compiler.
// supportedBasePlatforms is the list of base platform IDs the compiler supports (e.g., ["desktop"] or ["js"]).
// Returns the first platform ID from the target that matches a supported base platform.
// Returns empty string if no match found.
func (t *Target) SelectPlatformForCompiler(supportedBasePlatforms []string) string {
	if t == nil || len(supportedBasePlatforms) == 0 {
		return ""
	}

	supportedSet := make(map[string]struct{}, len(supportedBasePlatforms))
	for _, bp := range supportedBasePlatforms {
		supportedSet[bp] = struct{}{}
	}

	for _, platformID := range t.PlatformIDs {
		platform, err := ParsePlatform(platformID)
		if err != nil {
			continue
		}
		basePlatformID := platform.GetBasePlatformID()
		if _, ok := supportedSet[basePlatformID]; ok {
			return platformID
		}
	}

	return ""
}

// ListBuiltinTargetIDs returns a list of all builtin target IDs.
func ListBuiltinTargetIDs() []string {
	ids := make([]string, 0, len(BuiltinTargets)+1)
	for id := range BuiltinTargets {
		ids = append(ids, id)
	}
	ids = append(ids, TargetID_Desktop)
	return ids
}
