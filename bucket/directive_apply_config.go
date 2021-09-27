package bucket

import (
	"errors"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/golang/protobuf/proto"
	"regexp"
	"strconv"
)

// ApplyBucketConfig is a directive to apply a bucket configuration.
type ApplyBucketConfig interface {
	// Directive indicates ApplyBucketConfig is a directive.
	directive.Directive

	// ApplyBucketConfigBucketConf returns the desired bucket config.
	// The configuration with the highest revision will be applied.
	// Cannot be empty.
	ApplyBucketConfigBucketConf() *Config
	// ApplyBucketConfigVolumeIDRe returns the volume ID constraint.
	// Can be empty to select only volumes that already have the bucket.
	ApplyBucketConfigVolumeIDRe() *regexp.Regexp
}

// ApplyBucketConfigValue is the result type for ApplyBucketConfig.
type ApplyBucketConfigValue = *ApplyBucketConfigResult

/*
	// GetVolumeId returns the volume ID for this apply event.
	GetVolumeId() string
	// GetBucketId returns the bucket ID for this apply event.
	GetBucketId() string
	// GetBucketConf returns the bucket configuration applied.
	GetBucketConf() *Config
	// GetOldBucketConf returns the previous bucket configuration.
	GetOldBucketConf() *Config
	// GetTimestamp returns the timestamp of the event.
	GetTimestamp() *timestamp.Timestamp
	// GetUpdated indicates if the config was updated or not
	GetUpdated() bool
*/

// applyBucketConfig implements ApplyBucketConfig.
type applyBucketConfig struct {
	bucketConf *Config
	volumeIDRe *regexp.Regexp
}

// NewApplyBucketConfig constructs an ApplyBucketConfig.
func NewApplyBucketConfig(bucketConf *Config, volumeIDRe *regexp.Regexp) ApplyBucketConfig {
	return &applyBucketConfig{bucketConf: bucketConf, volumeIDRe: volumeIDRe}
}

// NewApplyBucketConfigToVolume constructs an ApplyBucketConfig with a regex matching a volume ID exactly.
func NewApplyBucketConfigToVolume(bucketConf *Config, volumeID string) ApplyBucketConfig {
	return NewApplyBucketConfig(bucketConf, regexp.MustCompile(regexp.QuoteMeta(volumeID)))
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
// Can be empty.
func (d *applyBucketConfig) ApplyBucketConfigVolumeIDRe() *regexp.Regexp {
	return d.volumeIDRe
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

	if !proto.Equal(d.ApplyBucketConfigBucketConf(), od.ApplyBucketConfigBucketConf()) {
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
		strconv.FormatUint(uint64(d.ApplyBucketConfigBucketConf().GetVersion()), 10),
	}
	if vre := d.ApplyBucketConfigVolumeIDRe(); vre != nil {
		vals["volume-id-regex"] = []string{vre.String()}
	}
	return vals
}

// _ is a type assertion
var _ ApplyBucketConfig = ((*applyBucketConfig)(nil))
