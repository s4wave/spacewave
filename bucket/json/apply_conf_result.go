package bucket_json

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/bucket"
)

// ApplyBucketConfigResult is the JSON marshaler for the response.
type ApplyBucketConfigResult struct {
	// BucketId is the bucket ID.
	BucketId string `json:"bucket_id"`
	// VolumeId is the volume ID.
	VolumeId string `json:"volume_id"`
	// Error is the error.
	Error string `json:"error"`
	// BucketConf is the curr bucket conf.
	BucketConf *Config `json:"bucket_conf"`
	// OldBucketConf is the old bucket conf.
	OldBucketConf *Config `json:"old_bucket_conf"`
	// Timestamp is the timestamp.
	Timestamp time.Time `json:"timestamp"`
	// Updated indicates if the value was updated.
	Updated bool `json:"updated"`
}

// NewApplyBucketConfigResult builds a new put bucket config response.
func NewApplyBucketConfigResult(
	ctx context.Context,
	b bus.Bus,
	obj *bucket.ApplyBucketConfigResult,
) (*ApplyBucketConfigResult, error) {
	bc, err := NewConfig(ctx, b, obj.GetBucketConf())
	if err != nil {
		return nil, err
	}

	obc, err := NewConfig(ctx, b, obj.GetOldBucketConf())
	if err != nil {
		return nil, err
	}

	return &ApplyBucketConfigResult{
		BucketId:      obj.GetBucketId(),
		VolumeId:      obj.GetVolumeId(),
		Error:         obj.GetError(),
		BucketConf:    bc,
		OldBucketConf: obc,
		Timestamp:     obj.GetTimestamp().ToTime(),
		Updated:       obj.GetUpdated(),
	}, nil
}
