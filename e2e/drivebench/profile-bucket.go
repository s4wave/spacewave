//go:build !js

package drivebench

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
)

// ProfileBucket is one source/function bucket from the browser JS CPU profile.
type ProfileBucket struct {
	// Name is the source or function the bucket covers.
	Name string `json:"name"`
	// Count is the number of samples in the bucket.
	Count int `json:"count,omitempty"`
	// SelfUs is the time spent in the bucket itself, in microseconds.
	SelfUs int64 `json:"selfUs,omitempty"`
	// TotalUs is the time including callees, in microseconds.
	TotalUs int64 `json:"totalUs,omitempty"`
}

func marshalProfileBucketValue(arena *fastjson.Arena, bucket ProfileBucket) *fastjson.Value {
	// Encode the browser profile bucket's name and recorded timing.
	obj := arena.NewObject()
	obj.Set("name", arena.NewString(bucket.Name))
	if bucket.Count != 0 {
		obj.Set("count", arena.NewNumberInt(bucket.Count))
	}
	if bucket.SelfUs != 0 {
		obj.Set("selfUs", arena.NewNumberString(strconv.FormatInt(bucket.SelfUs, 10)))
	}
	if bucket.TotalUs != 0 {
		obj.Set("totalUs", arena.NewNumberString(strconv.FormatInt(bucket.TotalUs, 10)))
	}
	return obj
}
