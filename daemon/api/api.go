package api

import (
	"errors"
	"regexp"
)

// Validate validates the request.
func (r *PutBlockRequest) Validate() error {
	if r.GetBucketId() == "" {
		return errors.New("bucket id must be specified")
	}
	if len(r.GetData()) == 0 {
		return errors.New("empty blocks not allowed")
	}
	if err := r.GetPutOpts().Validate(); err != nil {
		return err
	}
	return nil
}

// Validate validates the request.
func (r *GetBlockRequest) Validate() error {
	if r.GetBucketId() == "" {
		return errors.New("bucket id cannot be empty")
	}
	if _, err := r.ParseVolumeIDRe(); err != nil {
		return err
	}
	if err := r.GetBlockRef().Validate(); err != nil {
		return err
	}
	return nil
}

// ParseVolumeIDRe parses the volume id regex field.
func (a *GetBlockRequest) ParseVolumeIDRe() (*regexp.Regexp, error) {
	vre := a.GetVolumeIdRe()
	if vre == "" {
		return nil, nil
	}
	return regexp.Compile(vre)
}
