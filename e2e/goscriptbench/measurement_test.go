//go:build !js

package goscriptbench

import "testing"

func TestMeasurementRequiresSeparatedDiagnosticEvidence(t *testing.T) {
	metadata := testRunMetadata()
	untracedRequest := SampleRequest{Kind: SampleKindRetained, Number: 1}
	untraced := Measurement{Sample: testSample("retained-1", 10, false)}
	if err := untraced.Validate(untracedRequest, metadata); err != nil {
		t.Fatal(err.Error())
	}

	tracedRequest := SampleRequest{Kind: SampleKindDiagnostic, Number: 1, Trace: true}
	traced := Measurement{
		Sample:       testSample("diagnostic-1", 12, true),
		RuntimeTrace: []byte("runtime trace"),
	}
	if err := traced.Validate(tracedRequest, metadata); err != nil {
		t.Fatal(err.Error())
	}

	invalid := untraced
	invalid.RuntimeTrace = []byte("scalar trace")
	if err := invalid.Validate(untracedRequest, metadata); err == nil {
		t.Fatal("untraced measurement accepted diagnostic evidence")
	}
	invalid = traced
	invalid.RuntimeTrace = nil
	if err := invalid.Validate(tracedRequest, metadata); err == nil {
		t.Fatal("traced measurement accepted no runtime trace")
	}
	invalid = traced
	invalid.BrowserCPUProfile = []byte("profile")
	firefoxMetadata := metadata
	firefoxMetadata.Engine = "firefox"
	if err := invalid.Validate(tracedRequest, firefoxMetadata); err == nil {
		t.Fatal("Firefox measurement accepted a Chromium CPU profile")
	}
}
