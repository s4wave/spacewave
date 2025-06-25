package bldr_manifest

import (
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
)

// FetchManifest is a directive to fetch a manifest to storage.
//
// Value type: *FetchManifestValue
type FetchManifest interface {
	// Directive indicates FetchManifest is a directive.
	directive.Directive

	// GetManifestId returns the identifier of the manifest.
	GetManifestId() string
	// GetBuildTypes returns the build types to match, if empty match all.
	GetBuildTypes() []BuildType
	// GetPlatformIds returns the platform IDs to match, if empty match any.
	GetPlatformIds() []string
	// GetRev returns the minimum revision number of the manifest(s) to accept.
	// If set to 0, match any.
	GetRev() uint64
}

// fetchManifest implements FetchManifest
type fetchManifest struct {
	// manifestId is the identifier of the manifest.
	manifestId string
	// buildTypes is a slice of BuildType which indicates which build types to match, if empty match all.
	buildTypes []BuildType
	// platformIds is a slice of strings with platform IDs to match, if empty match any.
	platformIds []string
	// rev is the minimum revision number of the manifest(s) to accept. If set to 0, match any.
	rev uint64
}

// NewFetchManifest constructs a new FetchManifest directive.
func NewFetchManifest(manifestId string, buildTypes []BuildType, platformIds []string, rev uint64) FetchManifest {
	return &fetchManifest{
		manifestId:  manifestId,
		buildTypes:  buildTypes,
		platformIds: platformIds,
		rev:         rev,
	}
}

// NewFetchManifestValue constructs a new FetchManifest result value.
func NewFetchManifestValue(manifestRefs []*ManifestRef) *FetchManifestValue {
	return &FetchManifestValue{
		ManifestRefs: manifestRefs,
	}
}

// NewFetchManifestBuildMatrix constructs a slice of ManifestMeta for each combination
// of build type and platform ID specified in the directive.
// Returns one meta per build type x platform ID combination.
func NewFetchManifestBuildMatrix(directive FetchManifest) []*ManifestMeta {
	buildTypes := directive.GetBuildTypes()
	platformIds := directive.GetPlatformIds()
	
	// If no platform IDs specified, return nil
	if len(platformIds) == 0 {
		return nil
	}
	
	// Determine the build type to use
	var selectedBuildType BuildType
	if len(buildTypes) == 0 {
		// If no build type specified, use BuildType_DEV
		selectedBuildType = BuildType_DEV
	} else {
		// Check if both DEV and RELEASE are present
		hasDev := false
		hasRelease := false
		for _, bt := range buildTypes {
			if bt == BuildType_DEV {
				hasDev = true
			}
			if bt == BuildType_RELEASE {
				hasRelease = true
			}
		}
		
		if hasDev && hasRelease {
			// If both DEV and RELEASE are set, use RELEASE
			selectedBuildType = BuildType_RELEASE
		} else {
			// Otherwise use the first from the build types slice
			selectedBuildType = buildTypes[0]
		}
	}
	
	var metas []*ManifestMeta
	for _, platformId := range platformIds {
		meta := NewManifestMeta(
			directive.GetManifestId(),
			selectedBuildType,
			platformId,
			0, // Ignore GetRev field, set to zero
		)
		metas = append(metas, meta)
	}
	
	return metas
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *fetchManifest) Validate() error {
	if d.manifestId == "" {
		return ErrEmptyManifestID
	}

	return nil
}

// GetValueFetchManifestOptions returns options relating to value handling.
func (d *fetchManifest) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		// UnrefDisposeDur is the duration to wait to dispose a directive after all
		// references have been released.
		UnrefDisposeDur:            time.Second * 1,
		UnrefDisposeEmptyImmediate: true,
	}
}

// GetManifestId returns the identifier of the manifest.
func (d *fetchManifest) GetManifestId() string {
	return d.manifestId
}

// GetBuildTypes returns the build types to match, if empty match all.
func (d *fetchManifest) GetBuildTypes() []BuildType {
	return d.buildTypes
}

// GetPlatformIds returns the platform IDs to match, if empty match any.
func (d *fetchManifest) GetPlatformIds() []string {
	return d.platformIds
}

// GetRev returns the minimum revision number of the manifest(s) to accept.
// If set to 0, match any.
func (d *fetchManifest) GetRev() uint64 {
	return d.rev
}


// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *fetchManifest) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(FetchManifest)
	if !ok {
		return false
	}

	// Compare manifest IDs
	if d.GetManifestId() != od.GetManifestId() {
		return false
	}

	// Compare build types
	dBuildTypes, odBuildTypes := d.GetBuildTypes(), od.GetBuildTypes()
	if len(dBuildTypes) != len(odBuildTypes) {
		return false
	}
	for i, bt := range dBuildTypes {
		if bt != odBuildTypes[i] {
			return false
		}
	}

	// Compare platform IDs
	dPlatformIds, odPlatformIds := d.GetPlatformIds(), od.GetPlatformIds()
	if len(dPlatformIds) != len(odPlatformIds) {
		return false
	}
	for i, pid := range dPlatformIds {
		if pid != odPlatformIds[i] {
			return false
		}
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *fetchManifest) Superceeds(other directive.Directive) bool {
	od, ok := other.(FetchManifest)
	if !ok {
		return false
	}

	return d.GetRev() > od.GetRev()
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *fetchManifest) GetName() string {
	return "FetchManifest"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *fetchManifest) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["manifest-id"] = []string{d.GetManifestId()}

	if len(d.GetBuildTypes()) != 0 {
		buildTypeStrs := make([]string, len(d.GetBuildTypes()))
		for i, bt := range d.GetBuildTypes() {
			buildTypeStrs[i] = bt.String()
		}
		vals["build-types"] = buildTypeStrs
	}

	if len(d.GetPlatformIds()) != 0 {
		vals["platform-ids"] = d.GetPlatformIds()
	}

	if d.GetRev() != 0 {
		vals["rev"] = []string{string(rune(d.GetRev()))}
	}

	return vals
}

// _ is a type assertion
var _ FetchManifest = ((*fetchManifest)(nil))
