package bucket_setup

import (
	"errors"
	"regexp"

	"github.com/s4wave/spacewave/db/bucket"
)

// Validate checks the ApplyBucketConfig.
func (a *ApplyBucketConfig) Validate() error {
	if len(a.GetVolumeIdList()) != 0 && len(a.GetVolumeIdRe()) != 0 {
		return errors.New("volume id regex cannot be set if volume id list is set")
	}
	if _, err := a.ParseVolumeIdRe(); err != nil {
		return err
	}
	if err := a.GetConfig().Validate(); err != nil {
		return err
	}
	return nil
}

// ParseVolumeIdRe parses the volume id regex field.
// Returns nil if the field was empty.
func (a *ApplyBucketConfig) ParseVolumeIdRe() (*regexp.Regexp, error) {
	r := a.GetVolumeIdRe()
	if r == "" {
		return nil, nil
	}
	return regexp.Compile(r)
}

// BuildDirective builds a ApplyBucketConfig directive.
func (a *ApplyBucketConfig) BuildDirective() (bucket.ApplyBucketConfig, error) {
	r, err := a.ParseVolumeIdRe()
	if err != nil {
		return nil, err
	}
	dir := bucket.NewApplyBucketConfig(
		a.GetConfig(),
		r,
		a.GetVolumeIdList(),
	)
	if err := dir.Validate(); err != nil {
		return nil, err
	}
	return dir, nil
}
