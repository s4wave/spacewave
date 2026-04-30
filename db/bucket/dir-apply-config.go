//go:build !sql_lite

package bucket

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strconv"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// ApplyBucketConfig is a directive to apply a bucket configuration.
type ApplyBucketConfig interface {
	// Directive indicates ApplyBucketConfig is a directive.
	directive.Directive

	// ApplyBucketConfigBucketConf returns the desired bucket config.
	// The configuration with the highest rev will be applied.
	// Cannot be empty.
	ApplyBucketConfigBucketConf() *Config
	// ApplyBucketConfigVolumeIDRe returns the volume ID constraint.
	// Can be empty to select only volumes that already have the bucket.
	// Cannot be specified if VolumeIDList is set.
	ApplyBucketConfigVolumeIDRe() *regexp.Regexp
	// ApplyBucketConfigVolumeIDList returns a specific list of volumes to apply to.
	// If empty, uses the VolumeIDRe field instead.
	// Cannot be specified if VolumeIDRe is set.
	ApplyBucketConfigVolumeIDList() []string
}

// ApplyBucketConfigValue is the result type for ApplyBucketConfig.
type ApplyBucketConfigValue = *ApplyBucketConfigResult

// applyBucketConfig implements ApplyBucketConfig.
type applyBucketConfig struct {
	bucketConf   *Config
	volumeIDRe   *regexp.Regexp
	volumeIDList []string
}

// NewApplyBucketConfig constructs an ApplyBucketConfig.
func NewApplyBucketConfig(
	bucketConf *Config,
	volumeIDRe *regexp.Regexp,
	volumeIDList []string,
) ApplyBucketConfig {
	return &applyBucketConfig{
		bucketConf:   bucketConf,
		volumeIDRe:   volumeIDRe,
		volumeIDList: volumeIDList,
	}
}

// NewApplyBucketConfigToVolume constructs an ApplyBucketConfig with the volume id.
func NewApplyBucketConfigToVolume(bucketConf *Config, volumeID string) ApplyBucketConfig {
	return NewApplyBucketConfig(bucketConf, nil, []string{volumeID})
}

// NewApplyBucketConfigToVolumes constructs an ApplyBucketConfig with a list of volume ids.
func NewApplyBucketConfigToVolumes(bucketConf *Config, volumeIDs []string) ApplyBucketConfig {
	vids := make([]string, len(volumeIDs))
	copy(vids, volumeIDs)
	return NewApplyBucketConfig(bucketConf, nil, vids)
}

// ExApplyBucketConfig executes applying a bucket config directive.
func ExApplyBucketConfig(ctx context.Context, b bus.Bus, apply ApplyBucketConfig) (ApplyBucketConfigValue, error) {
	av, _, avRel, err := bus.ExecOneOff(ctx, b, apply, nil, nil)
	if err != nil {
		return nil, err
	}
	avRel.Release()
	val, ok := av.GetValue().(ApplyBucketConfigValue)
	if !ok {
		return nil, errors.New("apply bucket config: unexpected value")
	}
	if errStr := val.GetError(); errStr != "" {
		err = errors.New(errStr)
	}
	return val, err
}

// CheckApplyBucketConfigMatchesVolume checks if the directive matches the volume.
// volID is the primary volume ID.
// alias is a list of any alias volume IDs for volID.
func CheckApplyBucketConfigMatchesVolume(dir ApplyBucketConfig, volID string, alias []string) bool {
	if volumeIDConstraint := dir.ApplyBucketConfigVolumeIDRe(); volumeIDConstraint != nil {
		if volumeIDConstraint.MatchString(volID) {
			return true
		}
		return slices.ContainsFunc(alias, volumeIDConstraint.MatchString)
	}
	if volumeIDList := dir.ApplyBucketConfigVolumeIDList(); len(volumeIDList) != 0 {
		var matched bool
		for _, desiredID := range volumeIDList {
			if matched = desiredID == volID; matched {
				break
			}
			for _, aliasID := range alias {
				if matched = desiredID == aliasID; matched {
					break
				}
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *applyBucketConfig) Validate() error {
	if d.bucketConf == nil {
		return errors.New("bucket config cannot be empty")
	}
	if err := d.bucketConf.Validate(); err != nil {
		return err
	}
	if len(d.volumeIDList) != 0 {
		if d.volumeIDRe != nil {
			return errors.New("volume id regex cannot be set if volume id list is set")
		}
	}

	return nil
}

// GetValueApplyBucketConfigOptions returns options relating to value handling.
func (d *applyBucketConfig) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// ApplyBucketConfigBucketConf returns the bucket config.
func (d *applyBucketConfig) ApplyBucketConfigBucketConf() *Config {
	return d.bucketConf
}

// ApplyBucketConfigVolumeIDRe returns the volume ID constraint.
// Cannot be specified if VolumeIDList is set.
// Can be empty to select only volumes that already have the bucket.
// If VolumeIDList is set, it will override this field.
func (d *applyBucketConfig) ApplyBucketConfigVolumeIDRe() *regexp.Regexp {
	return d.volumeIDRe
}

// ApplyBucketConfigVolumeIDList returns a specific list of volumes to apply to.
// Cannot be specified if VolumeIDRe is set.
// If empty, uses the VolumeIDRe field instead.
func (d *applyBucketConfig) ApplyBucketConfigVolumeIDList() []string {
	return d.volumeIDList
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *applyBucketConfig) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(ApplyBucketConfig)
	if !ok {
		return false
	}

	var vid1s, vid2s string
	if vid1 := d.ApplyBucketConfigVolumeIDRe(); vid1 != nil {
		vid1s = vid1.String()
	}
	if vid2 := od.ApplyBucketConfigVolumeIDRe(); vid2 != nil {
		vid2s = vid2.String()
	}
	if vid1s != vid2s {
		return false
	}

	volIds1 := d.ApplyBucketConfigVolumeIDList()
	volIds2 := od.ApplyBucketConfigVolumeIDList()
	if !slices.Equal(volIds1, volIds2) {
		return false
	}

	if !d.ApplyBucketConfigBucketConf().EqualVT(od.ApplyBucketConfigBucketConf()) {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *applyBucketConfig) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *applyBucketConfig) GetName() string {
	return "ApplyBucketConfig"
}

// GetDebugString returns the directive arguments stringified.
func (d *applyBucketConfig) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["bucket-id"] = []string{d.ApplyBucketConfigBucketConf().GetId()}
	vals["bucket-conf-rev"] = []string{
		strconv.FormatUint(uint64(d.ApplyBucketConfigBucketConf().GetRev()), 10),
	}
	if vre := d.ApplyBucketConfigVolumeIDRe(); vre != nil {
		vals["volume-id-regex"] = []string{vre.String()}
	}
	if vre := d.ApplyBucketConfigVolumeIDList(); vre != nil {
		vals["volume-id"] = vre
	}
	return vals
}

// _ is a type assertion
var _ ApplyBucketConfig = ((*applyBucketConfig)(nil))
