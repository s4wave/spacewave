//go:build !js

package goscriptbench

import (
	"math"

	"github.com/pkg/errors"
)

// Sample records one browser-local projected-image measurement.
type Sample struct {
	// ID uniquely identifies the sample within one engine run
	ID string
	// requestStartMs is the projected URL assignment time relative to sample start
	RequestStartMs float64
	// responseStartMs is the matching resource response start
	ResponseStartMs float64
	// responseEndMs is the matching resource response end
	ResponseEndMs float64
	// loadMs is the image load event time
	LoadMs float64
	// decodeMs is the image decode completion time
	DecodeMs float64
	// frameMs is the first animation frame after decode
	FrameMs float64
	// displayReadyMs is the retained scalar and equals frameMs
	DisplayReadyMs float64
	// naturalWidth is the decoded image width
	NaturalWidth int
	// naturalHeight is the decoded image height
	NaturalHeight int
	// transferSize is the browser-reported transferred byte count
	TransferSize int64
	// decodedBodySize is the browser-reported decoded body byte count
	DecodedBodySize int64
	// traced reports whether diagnostic tracing was enabled
	Traced bool
}

// Validate checks that the sample is complete for the run metadata.
func (s Sample) Validate(metadata RunMetadata) error {
	if s.ID == "" {
		return errors.New("sample ID is required")
	}
	timings := []struct {
		name  string
		value float64
	}{
		{name: "requestStartMs", value: s.RequestStartMs},
		{name: "responseStartMs", value: s.ResponseStartMs},
		{name: "responseEndMs", value: s.ResponseEndMs},
		{name: "loadMs", value: s.LoadMs},
		{name: "decodeMs", value: s.DecodeMs},
		{name: "frameMs", value: s.FrameMs},
		{name: "displayReadyMs", value: s.DisplayReadyMs},
	}
	for _, timing := range timings {
		if math.IsNaN(timing.value) || math.IsInf(timing.value, 0) || timing.value < 0 {
			return errors.Errorf("sample %q has invalid %s", s.ID, timing.name)
		}
	}
	responseStartUnavailable := metadata.fieldUnavailable("responseStartMs")
	if responseStartUnavailable && s.ResponseStartMs != 0 {
		return errors.Errorf("sample %q reports unavailable responseStartMs", s.ID)
	}
	if !responseStartUnavailable && s.ResponseStartMs < s.RequestStartMs {
		return errors.Errorf("sample %q response starts before its request", s.ID)
	}
	responseEndUnavailable := metadata.fieldUnavailable("responseEndMs")
	if responseEndUnavailable && s.ResponseEndMs != 0 {
		return errors.Errorf("sample %q reports unavailable responseEndMs", s.ID)
	}
	if !responseEndUnavailable && s.ResponseEndMs < s.ResponseStartMs {
		return errors.Errorf("sample %q response ends before it starts", s.ID)
	}
	if s.LoadMs <= s.RequestStartMs || s.DecodeMs < s.LoadMs || s.FrameMs < s.DecodeMs {
		return errors.Errorf("sample %q has an incomplete display timeline", s.ID)
	}
	if s.DisplayReadyMs != s.FrameMs {
		return errors.Errorf("sample %q displayReadyMs differs from frameMs", s.ID)
	}
	if s.NaturalWidth != metadata.Fixture.Width || s.NaturalHeight != metadata.Fixture.Height {
		return errors.Errorf("sample %q dimensions differ from the fixture", s.ID)
	}
	if s.TransferSize < 0 || s.DecodedBodySize < 0 {
		return errors.Errorf("sample %q has a negative resource size", s.ID)
	}
	if metadata.fieldUnavailable("transferSize") && s.TransferSize != 0 {
		return errors.Errorf("sample %q reports unavailable transferSize", s.ID)
	}
	if metadata.fieldUnavailable("decodedBodySize") && s.DecodedBodySize != 0 {
		return errors.Errorf("sample %q reports unavailable decodedBodySize", s.ID)
	}
	return nil
}
