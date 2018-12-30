package api

import (
	"errors"
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
