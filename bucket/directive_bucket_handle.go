package bucket

import (
	"errors"
	"github.com/aperturerobotics/controllerbus/directive"
	"regexp"
)

// BuildBucketAPI is a directive to get API handles to buckets.
type BuildBucketAPI interface {
	// Directive indicates BuildBucketAPI is a directive.
	directive.Directive

	// BuildBucketAPIBucketID returns the bucket ID constraint.
	// Cannot be empty.
	BuildBucketAPIBucketID() string
	// BuildBucketAPIVolumeIDRe returns the volume ID constraint.
	// Can be empty.
	BuildBucketAPIVolumeIDRe() *regexp.Regexp
}

// BuildBucketAPIValue is the result type for BuildBucketAPI.
// The value is removed and replaced when any values change.
type BuildBucketAPIValue = Bucket

// buildBucketAPI implements BuildBucketAPI
type buildBucketAPI struct {
	bucketID   string
	volumeIDRe *regexp.Regexp
}

// BuildBucketAPIOption is a directive option as a function.
type BuildBucketAPIOption = func(b *buildBucketAPI) error

// NewBuildBucketAPI constructs a new BuildBucketAPI directive.
func NewBuildBucketAPI(opts ...BuildBucketAPIOption) (BuildBucketAPI, error) {
	c := &buildBucketAPI{}
	for _, v := range opts {
		if err := v(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *buildBucketAPI) Validate() error {
	if d.bucketID == "" {
		return errors.New("bucket id cannot be empty")
	}

	return nil
}

// GetValueBuildBucketAPIOptions returns options relating to value handling.
func (d *buildBucketAPI) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// BuildBucketAPIBucketIDRe returns the bucket ID constraint.
func (d *buildBucketAPI) BuildBucketAPIBucketID() string {
	return d.bucketID
}

// BuildBucketAPIVolumeIDRe returns the volume ID constraint.
// Can be empty.
func (d *buildBucketAPI) BuildBucketAPIVolumeIDRe() *regexp.Regexp {
	return d.volumeIDRe
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *buildBucketAPI) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(BuildBucketAPI)
	if !ok {
		return false
	}

	var vid1s, vid2s string
	if vid1 := d.BuildBucketAPIVolumeIDRe(); vid1 != nil {
		vid1s = vid1.String()
	}
	if vid2 := od.BuildBucketAPIVolumeIDRe(); vid2 != nil {
		vid2s = vid2.String()
	}
	if vid1s != vid2s {
		return false
	}

	if d.BuildBucketAPIBucketID() != od.BuildBucketAPIBucketID() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *buildBucketAPI) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *buildBucketAPI) GetName() string {
	return "BuildBucketAPI"
}

// GetDebugString returns the directive arguments stringified.
// This should be something like param1="test", param2="test".
// This is not necessarily unique, and is primarily intended for display.
func (d *buildBucketAPI) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	vals["bucket-id"] = []string{d.BuildBucketAPIBucketID()}
	if vre := d.BuildBucketAPIVolumeIDRe(); vre != nil {
		vals["volume-id-regex"] = []string{vre.String()}
	}
	return vals
}

// _ is a type assertion
var _ BuildBucketAPI = ((*buildBucketAPI)(nil))
