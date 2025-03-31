package volume

import (
	"regexp"
	"slices"

	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
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
	// Cannot be specified if VolumeIDList is set.
	ListBucketsVolumeIDRe() *regexp.Regexp
	// ListBucketsVolumeIDList returns a specific list of volumes to list.
	// If empty, uses the VolumeIDRe field instead.
	// Cannot be specified if VolumeIDRe is set.
	ListBucketsVolumeIDList() []string
}

// ListBucketsValue is the result type for ListBuckets.
type ListBucketsValue = VolumeBucketInfo

// NewListBuckets constructs a ListBuckets with a list of volumes.
func NewListBuckets(bucketID string, volumeIDs []string) ListBuckets {
	volIDs := make([]string, len(volumeIDs))
	copy(volIDs, volumeIDs)
	return &ListBucketsRequest{
		BucketId:     bucketID,
		VolumeIdList: volIDs,
	}
}

// NewListBucketsWithRe constructs an ListBuckets with a regexp.
func NewListBucketsWithRe(bucketID string, volumeIDRe string) ListBuckets {
	return &ListBucketsRequest{
		BucketId:   bucketID,
		VolumeIdRe: volumeIDRe,
	}
}

// Validate validates the directive.
// This is a cursory validation to see if the values "look correct."
func (d *ListBucketsRequest) Validate() error {
	if d.GetVolumeIdRe() != "" {
		if len(d.GetVolumeIdList()) != 0 {
			return errors.New("volume_re and volume_id_list cannot both be set")
		}
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
// Cannot be specified if VolumeIDList is set.
func (a *ListBucketsRequest) ListBucketsVolumeIDRe() *regexp.Regexp {
	re, _ := confparse.ParseRegexp(a.GetVolumeIdRe())
	return re
}

// ParseVolumeIDRe parses the volume id regex field.
func (a *ListBucketsRequest) ParseVolumeIDRe() (*regexp.Regexp, error) {
	return confparse.ParseRegexp(a.GetVolumeIdRe())
}

// ListBucketsVolumeIDList returns a specific list of volumes to list.
// If empty, uses the VolumeIDRe field instead.
// Cannot be specified if VolumeIDRe is set.
func (a *ListBucketsRequest) ListBucketsVolumeIDList() []string {
	ids := a.GetVolumeIdList()
	out := make([]string, len(ids))
	copy(out, ids)
	return out
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

	if !slices.Equal(d.ListBucketsVolumeIDList(), od.ListBucketsVolumeIDList()) {
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
	if ids := d.ListBucketsVolumeIDList(); ids != nil {
		vals["volume-id-list"] = ids
	}
	return vals
}

// _ is a type assertion
var _ ListBuckets = ((*ListBucketsRequest)(nil))
