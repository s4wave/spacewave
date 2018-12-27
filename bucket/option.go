package bucket

import (
	"regexp"
)

// VolumeIDRegex configures the directive to limit to volume IDs by a regular
// expression.
func VolumeIDRegex(r *regexp.Regexp) BuildBucketAPIOption {
	return func(c *buildBucketAPI) error {
		c.volumeIDRe = r
		return nil
	}
}

// WithVolumeID configures the directive to limit to a specific volume ID.
func WithVolumeID(id string) BuildBucketAPIOption {
	return func(c *buildBucketAPI) (err error) {
		c.volumeIDRe, err = regexp.Compile("^" + regexp.QuoteMeta(id) + "$")
		return
	}
}

// WithBucketID configures the directive to limit to a specific bucket ID.
func WithBucketID(id string) BuildBucketAPIOption {
	return func(c *buildBucketAPI) error {
		c.bucketID = id
		return nil
	}
}
