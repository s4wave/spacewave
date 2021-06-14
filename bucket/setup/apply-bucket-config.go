package bucket_setup

import (
	"regexp"

	"github.com/aperturerobotics/hydra/bucket"
)

// Validate checks the ApplyBucketConfig.
func (a *ApplyBucketConfig) Validate() error {
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
	)
	if err := dir.Validate(); err != nil {
		return nil, err
	}
	return dir, nil
}
