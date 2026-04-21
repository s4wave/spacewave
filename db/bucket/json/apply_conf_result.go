package bucket_json

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
)

// ApplyBucketConfigResult is the JSON marshaler for the response.
type ApplyBucketConfigResult struct {
	// BucketId is the bucket ID.
	BucketId string `json:"bucket_id,omitempty"`
	// VolumeId is the volume ID.
	VolumeId string `json:"volume_id,omitempty"`
	// Error is the error.
	Error string `json:"error,omitempty"`
	// BucketConf is the curr bucket conf.
	BucketConf *Config `json:"bucket_conf,omitempty"`
	// OldBucketConf is the old bucket conf.
	OldBucketConf *Config `json:"old_bucket_conf,omitempty"`
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
		Timestamp:     obj.GetTimestamp().AsTime(),
		Updated:       obj.GetUpdated(),
	}, nil
}

// MarshalJSON marshals the result to JSON.
func (c *ApplyBucketConfigResult) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}

	var a fastjson.Arena
	obj := a.NewObject()
	if c.BucketId != "" {
		obj.Set("bucket_id", a.NewString(c.BucketId))
	}
	if c.VolumeId != "" {
		obj.Set("volume_id", a.NewString(c.VolumeId))
	}
	if c.Error != "" {
		obj.Set("error", a.NewString(c.Error))
	}
	if c.BucketConf != nil {
		dat, err := c.BucketConf.MarshalJSON()
		if err != nil {
			return nil, errors.Wrap(err, "marshal bucket config")
		}
		bucketConf, err := marshalJSONBytesValue(&a, dat)
		if err != nil {
			return nil, errors.Wrap(err, "parse bucket config")
		}
		obj.Set("bucket_conf", bucketConf)
	}
	if c.OldBucketConf != nil {
		dat, err := c.OldBucketConf.MarshalJSON()
		if err != nil {
			return nil, errors.Wrap(err, "marshal old bucket config")
		}
		oldBucketConf, err := marshalJSONBytesValue(&a, dat)
		if err != nil {
			return nil, errors.Wrap(err, "parse old bucket config")
		}
		obj.Set("old_bucket_conf", oldBucketConf)
	}

	timestamp, err := c.Timestamp.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "marshal timestamp")
	}
	timestampValue, err := marshalJSONBytesValue(&a, timestamp)
	if err != nil {
		return nil, errors.Wrap(err, "parse timestamp")
	}
	obj.Set("timestamp", timestampValue)

	if c.Updated {
		obj.Set("updated", a.NewTrue())
	} else {
		obj.Set("updated", a.NewFalse())
	}
	return obj.MarshalTo(nil), nil
}
