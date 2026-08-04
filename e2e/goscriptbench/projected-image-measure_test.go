//go:build !js

package goscriptbench

import (
	"maps"
	"testing"
)

func TestProjectedImageUntracedSampleID(t *testing.T) {
	for _, test := range []struct {
		request SampleRequest
		want    string
	}{
		{
			request: SampleRequest{Kind: SampleKindWarmup, Number: 1},
			want:    "warmup-1",
		},
		{
			request: SampleRequest{Kind: SampleKindRetained, Number: 1},
			want:    "retained-1",
		},
		{
			request: SampleRequest{Kind: SampleKindRetained, Number: RetainedSampleCount},
			want:    "retained-10",
		},
	} {
		got, err := projectedImageUntracedSampleID(test.request)
		if err != nil {
			t.Fatal(err.Error())
		}
		if got != test.want {
			t.Fatalf("sample ID = %q, want %q", got, test.want)
		}
	}

	for _, request := range []SampleRequest{
		{Kind: SampleKindWarmup, Number: 2},
		{Kind: SampleKindRetained, Number: 0},
		{Kind: SampleKindRetained, Number: RetainedSampleCount + 1},
		{Kind: SampleKindDiagnostic, Number: 1},
		{Kind: SampleKindWarmup, Number: 1, Trace: true},
		{Kind: SampleKind("unknown"), Number: 1},
	} {
		if _, err := projectedImageUntracedSampleID(request); err == nil {
			t.Fatalf("invalid sample request validated: %+v", request)
		}
	}
}

func TestProjectedImageBrowserSampleRejectsInvalidEvidence(t *testing.T) {
	valid := testProjectedImageBrowserSample()
	result, err := projectedImageSampleFromBrowser(valid)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := result.validateRequest("retained-1", "https://example.test/projected.png?sample=1"); err != nil {
		t.Fatal(err.Error())
	}
	if err := result.sample.Validate(testRunMetadata()); err != nil {
		t.Fatal(err.Error())
	}

	for _, count := range []int{0, 2} {
		invalid := maps.Clone(valid)
		invalid["resourceEntryCount"] = count
		result, err := projectedImageSampleFromBrowser(invalid)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := result.validateRequest("retained-1", "https://example.test/projected.png?sample=1"); err == nil {
			t.Fatalf("resource entry count %d validated", count)
		}
	}

	incomplete := maps.Clone(valid)
	incomplete["loadMs"] = 0.0
	result, err = projectedImageSampleFromBrowser(incomplete)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := result.sample.Validate(testRunMetadata()); err == nil {
		t.Fatal("incomplete projected-image timeline validated")
	}

	missing := maps.Clone(valid)
	delete(missing, "frameMs")
	if _, err := projectedImageSampleFromBrowser(missing); err == nil {
		t.Fatal("projected-image sample with a missing field decoded")
	}
}

func testProjectedImageBrowserSample() map[string]any {
	return map[string]any{
		"projectedUrl":       "https://example.test/projected.png?sample=1",
		"resourceEntryCount": 1,
		"id":                 "retained-1",
		"requestStartMs":     0.0,
		"responseStartMs":    1.0,
		"responseEndMs":      2.0,
		"loadMs":             3.0,
		"decodeMs":           4.0,
		"frameMs":            5.0,
		"displayReadyMs":     5.0,
		"naturalWidth":       1024,
		"naturalHeight":      1024,
		"transferSize":       4_198_217,
		"decodedBodySize":    4_198_217,
		"traced":             false,
	}
}
