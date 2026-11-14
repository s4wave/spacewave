//go:build !js

package drivebench

import (
	"github.com/aperturerobotics/fastjson"
)

// BrowserProfile records the optional same-window browser JS CPU/profile
// artifact. It is present only when the harness attempted the profile gate.
type BrowserProfile struct {
	// Captured reports whether a profile was captured.
	Captured bool `json:"captured"`
	// ProfilePath is the path of the raw profile.
	ProfilePath string `json:"profilePath,omitempty"`
	// SummaryPath is the path of the profile summary.
	SummaryPath string `json:"summaryPath,omitempty"`
	// CaptureWindow names the window the profile covers.
	CaptureWindow string `json:"captureWindow,omitempty"`
	// StartedAt is when the capture started.
	StartedAt string `json:"startedAt,omitempty"`
	// StoppedAt is when the capture stopped.
	StoppedAt string `json:"stoppedAt,omitempty"`
	// Bytes is the size of the raw profile.
	Bytes int `json:"bytes,omitempty"`
	// SkippedReason says why the capture was skipped, if it was.
	SkippedReason string `json:"skippedReason,omitempty"`
	// Buckets lists the heaviest source and function buckets.
	Buckets []ProfileBucket `json:"buckets,omitempty"`
}

func marshalBrowserProfileValue(arena *fastjson.Arena, profile BrowserProfile) *fastjson.Value {
	// Encode whether the browser profile was captured.
	obj := arena.NewObject()
	if profile.Captured {
		obj.Set("captured", arena.NewTrue())
	} else {
		obj.Set("captured", arena.NewFalse())
	}

	// Encode the browser profile's paths and capture window.
	setOmitEmptyStringJSONField(arena, obj, "profilePath", profile.ProfilePath)
	setOmitEmptyStringJSONField(arena, obj, "summaryPath", profile.SummaryPath)
	setOmitEmptyStringJSONField(arena, obj, "captureWindow", profile.CaptureWindow)
	setOmitEmptyStringJSONField(arena, obj, "startedAt", profile.StartedAt)
	setOmitEmptyStringJSONField(arena, obj, "stoppedAt", profile.StoppedAt)

	// Include the browser profile's size, skip reason, and summary buckets.
	if profile.Bytes != 0 {
		obj.Set("bytes", arena.NewNumberInt(profile.Bytes))
	}
	setOmitEmptyStringJSONField(arena, obj, "skippedReason", profile.SkippedReason)
	if len(profile.Buckets) != 0 {
		arr := arena.NewArray()
		for _, bucket := range profile.Buckets {
			arr.SetArrayItem(len(arr.GetArray()), marshalProfileBucketValue(arena, bucket))
		}
		obj.Set("buckets", arr)
	}
	return obj
}
