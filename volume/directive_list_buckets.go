package volume

import (
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"regexp"
)

// ListBuckets is a directive to list buckets.
type ListBuckets interface {
	// Directive indicates ListBuckets is a directive.
	directive.Directive

	// ListBucketsBucketId returns the desired bucket id.
	// Can be empty.
	ListBucketsBucketId() string
	// ListBucketsVolumeIDRe returns the volume ID constraint.
	// Can be empty.
	ListBucketsVolumeIDRe() *regexp.Regexp
}

// ListBucketsValue is the result type for ListBuckets.
type ListBucketsValue = VolumeBucketInfo

// NewListBuckets constructs an ListBuckets.
func NewListBuckets(bucketID string, volumeIDRe string) ListBuckets {
	return &ListBucketsRequest{
		BucketId: bucketID,
		VolumeRe: volumeIDRe,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *ListBucketsRequest) Validate() error {
	if d.GetVolumeRe() != "" {
		if _, err := d.ParseVolumeIDRe(); err != nil {
			return errors.Wrap(err, "parse volume id re")
		}
	}

	return nil
}

// GetValueListBucketsOptions returns options relating to value handling.
func (d *ListBucketsRequest) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{}
}

// ListBucketsBucketId returns the desired bucket id.
// Can be empty.
func (a *ListBucketsRequest) ListBucketsBucketId() string {
	return a.GetBucketId()
}

// ListBucketsVolumeIDRe returns the volume ID constraint.
// Can be empty.
func (a *ListBucketsRequest) ListBucketsVolumeIDRe() *regexp.Regexp {
	r, _ := a.ParseVolumeIDRe()
	return r
}

// ParseVolumeIDRe parses the volume id regex field.
func (a *ListBucketsRequest) ParseVolumeIDRe() (*regexp.Regexp, error) {
	vre := a.GetVolumeRe()
	if vre == "" {
		return nil, nil
	}
	return regexp.Compile(vre)
}

// IsEquivalent checks if the other directive is equivalent. If two
// directives are equivalent, and the new directive does not superceed the
// old, then the new directive will be merged (de-duplicated) into the old.
func (d *ListBucketsRequest) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(ListBuckets)
	if !ok {
		return false
	}

	var vid1s, vid2s string
	if vid1 := d.ListBucketsVolumeIDRe(); vid1 != nil {
		vid1s = vid1.String()
	}
	if vid2 := od.ListBucketsVolumeIDRe(); vid2 != nil {
		vid2s = vid2.String()
	}
	if vid1s != vid2s {
		return false
	}

	if d.ListBucketsBucketId() != od.ListBucketsBucketId() {
		return false
	}

	return true
}

// Superceeds checks if the directive overrides another.
// The other directive will be canceled if superceded.
func (d *ListBucketsRequest) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
// This is not necessarily unique, and is primarily intended for display.
func (d *ListBucketsRequest) GetName() string {
	return "ListBuckets"
}

// GetDebugString returns the directive arguments stringified.
func (d *ListBucketsRequest) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.ListBucketsBucketId() != "" {
		vals["bucket-id"] = []string{d.ListBucketsBucketId()}
	}
	if vre := d.ListBucketsVolumeIDRe(); vre != nil {
		vals["volume-id-regex"] = []string{vre.String()}
	}
	return vals
}

// _ is a type assertion
var _ ListBuckets = ((*ListBucketsRequest)(nil))
