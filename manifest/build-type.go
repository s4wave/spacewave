package bldr_manifest

import (
	"strings"

	"github.com/pkg/errors"
)

// BuildType defines if we are building a dev or release build.
type BuildType string

const (
	// BuildType_DEV is the development build type.
	BuildType_DEV BuildType = "dev"
	// BuildType_RELEASE is the release build type.
	BuildType_RELEASE BuildType = "release"
)

// BuildType_ALIASES are aliases to the BuildTypes.
var BuildType_ALIASES = map[string]BuildType{
	"development": BuildType_DEV,
	"prod":        BuildType_RELEASE,
	"production":  BuildType_RELEASE,
}

// ToBuildType formats a BuildTypeStr into a BuildType.
func ToBuildType(buildTypeStr string) BuildType {
	buildTypeStr = strings.ToLower(buildTypeStr)
	buildTypeStr = strings.TrimSpace(buildTypeStr)
	if alias, ok := BuildType_ALIASES[buildTypeStr]; ok {
		buildTypeStr = string(alias)
	}
	if buildTypeStr == "" {
		return BuildType_DEV
	}
	return BuildType(buildTypeStr)
}

// Validate checks if the BuildType is one of the known types.
func (t BuildType) Validate(allowEmpty bool) error {
	if t == "" {
		if allowEmpty {
			return nil
		}
		return ErrEmptyBuildType
	}
	switch t {
	case BuildType_DEV:
	case BuildType_RELEASE:
	default:
		return errors.Errorf("unknown build type: %s", string(t))
	}
	return nil
}

// IsDev checks if the BuildType is development.
func (t BuildType) IsDev() bool {
	return ToBuildType(string(t)) == BuildType_DEV
}

// IsRelease checks if the BuildType is release.
func (t BuildType) IsRelease() bool {
	return ToBuildType(string(t)) == BuildType_RELEASE
}
