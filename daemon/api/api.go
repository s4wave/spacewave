package api

import (
	"errors"
)

// Validate validates the request.
func (r *PutBlockRequest) Validate() error {
	if err := r.GetBucketOpArgs().Validate(); err != nil {
		return err
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
	if err := r.GetBucketOpArgs().Validate(); err != nil {
		return err
	}
	if err := r.GetBlockRef().Validate(); err != nil {
		return err
	}
	return nil
}
