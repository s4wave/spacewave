//go:build !js

package goscriptbench

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
)

var retainedTestValues = []float64{9, 1, 8, 2, 7, 3, 6, 4, 5, 10}

type deterministicWorkload struct {
	metadata RunMetadata
	events   []string
}

func (w *deterministicWorkload) Setup(context.Context) (RunMetadata, error) {
	w.events = append(w.events, "setup")
	return w.metadata, nil
}

func (w *deterministicWorkload) Restart(_ context.Context, request SampleRequest) error {
	w.events = append(w.events, "restart:"+sampleRequestName(request))
	return nil
}

func (w *deterministicWorkload) Measure(_ context.Context, request SampleRequest) (Measurement, error) {
	w.events = append(w.events, "measure:"+sampleRequestName(request))
	value := 11.0
	if request.Kind == SampleKindRetained {
		value = retainedTestValues[request.Number-1]
	}
	if request.Kind == SampleKindDiagnostic {
		value = 12
	}
	measurement := Measurement{Sample: testSample(sampleRequestName(request), value, request.Trace)}
	if request.Trace {
		measurement.RuntimeTrace = []byte("deterministic runtime trace")
	}
	return measurement, nil
}

func (w *deterministicWorkload) Validate(_ context.Context, request SampleRequest, sample Sample) error {
	w.events = append(w.events, "validate:"+sampleRequestName(request))
	if sample.ID != sampleRequestName(request) {
		return errors.New("sample identity differs from its request")
	}
	return nil
}

func TestRunnerPublishesPerEngineArtifact(t *testing.T) {
	runner, err := NewRunner(t.TempDir())
	if err != nil {
		t.Fatal(err.Error())
	}
	workload := &deterministicWorkload{metadata: testRunMetadata()}
	artifactDir, err := runner.Run(context.Background(), workload)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Require the workload lifecycle to stay behind the package-local seam.
	expectedEvents := []string{"setup"}
	for _, request := range testSampleRequests() {
		name := sampleRequestName(request)
		expectedEvents = append(expectedEvents, "restart:"+name, "measure:"+name, "validate:"+name)
	}
	if !slices.Equal(workload.events, expectedEvents) {
		t.Fatalf("runner events = %q, want %q", workload.events, expectedEvents)
	}

	// Require one self-contained published engine directory.
	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		t.Fatal(err.Error())
	}
	files := make([]string, len(entries))
	for idx, entry := range entries {
		files[idx] = entry.Name()
	}
	slices.Sort(files)
	if want := []string{artifactDiagnosticFile, artifactManifestFile, artifactResultFile, artifactRuntimeTraceFile}; !slices.Equal(files, want) {
		t.Fatalf("artifact files = %q, want %q", files, want)
	}
	bundle, err := ReadArtifact(artifactDir)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bundle.Result.Metadata.Engine != "chromium" || bundle.Diagnostic.Engine != "chromium" {
		t.Fatalf("published engine identity = %q/%q", bundle.Result.Metadata.Engine, bundle.Diagnostic.Engine)
	}
	if bundle.Result.Warmup.ID != "warmup-1" || bundle.Diagnostic.Sample.ID != "diagnostic-1" {
		t.Fatalf("warm-up/diagnostic identity = %q/%q", bundle.Result.Warmup.ID, bundle.Diagnostic.Sample.ID)
	}
	if string(bundle.RuntimeTrace) != "deterministic runtime trace" {
		t.Fatalf("runtime trace = %q", bundle.RuntimeTrace)
	}
	if len(bundle.Result.Samples) != len(retainedTestValues) {
		t.Fatalf("retained row count = %d, want %d", len(bundle.Result.Samples), len(retainedTestValues))
	}
	for idx, value := range retainedTestValues {
		sample := bundle.Result.Samples[idx]
		if sample.ID != "retained-"+strconv.Itoa(idx+1) || sample.DisplayReadyMs != value {
			t.Fatalf("retained row %d = %q/%v", idx+1, sample.ID, sample.DisplayReadyMs)
		}
	}
}

func TestArtifactRequiresTruthfulMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ArtifactBundle)
	}{
		{name: "engine", mutate: func(b *ArtifactBundle) { b.Result.Metadata.Engine = "" }},
		{name: "engine version", mutate: func(b *ArtifactBundle) { b.Result.Metadata.EngineVersion = "" }},
		{name: "compiler", mutate: func(b *ArtifactBundle) { b.Result.Metadata.Compiler = "" }},
		{name: "Spacewave revision", mutate: func(b *ArtifactBundle) { b.Result.Metadata.SpacewaveRevision = "" }},
		{name: "GoScript revision", mutate: func(b *ArtifactBundle) { b.Result.Metadata.GoScriptRevision = "" }},
		{name: "fixture", mutate: func(b *ArtifactBundle) { b.Result.Metadata.Fixture.SHA256 = "" }},
		{name: "build mode", mutate: func(b *ArtifactBundle) { b.Result.Metadata.BuildMode = "" }},
		{name: "worker mode", mutate: func(b *ArtifactBundle) { b.Result.Metadata.WorkerMode = "" }},
		{name: "storage backend", mutate: func(b *ArtifactBundle) { b.Result.Metadata.StorageBackend = "" }},
		{name: "retained state", mutate: func(b *ArtifactBundle) { b.Result.Metadata.State.Retained = nil }},
		{name: "recreated state", mutate: func(b *ArtifactBundle) { b.Result.Metadata.State.Recreated = nil }},
		{name: "sample count", mutate: func(b *ArtifactBundle) { b.Result.Sampling.RetainedSamples = 9 }},
		{name: "unavailable fields", mutate: func(b *ArtifactBundle) { b.Result.Metadata.UnavailableFields = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := validArtifactBundle(t)
			test.mutate(&bundle)
			if err := bundle.Validate(); err == nil {
				t.Fatal("incomplete artifact validated")
			}
		})
	}
}

func TestSamplingContractRetainsRawRows(t *testing.T) {
	samples := make([]Sample, len(retainedTestValues))
	for idx, value := range retainedTestValues {
		samples[idx] = testSample("retained-"+strconv.Itoa(idx+1), value, false)
	}
	before := make([]float64, len(samples))
	for idx := range samples {
		before[idx] = samples[idx].DisplayReadyMs
	}

	summary, err := SummarizeSamples(samples)
	if err != nil {
		t.Fatal(err.Error())
	}
	if summary.Method != SummaryMethodNearestRank || summary.SampleCount != RetainedSampleCount {
		t.Fatalf("summary contract = %q/%d", summary.Method, summary.SampleCount)
	}
	if summary.MedianDisplayReadyMs != 5 || summary.P95DisplayReadyMs != 10 {
		t.Fatalf("nearest-rank p50/p95 = %v/%v, want 5/10", summary.MedianDisplayReadyMs, summary.P95DisplayReadyMs)
	}
	for idx := range samples {
		if samples[idx].DisplayReadyMs != before[idx] {
			t.Fatalf("source row %d moved from %v to %v", idx, before[idx], samples[idx].DisplayReadyMs)
		}
	}
	policy := fixedSamplingPolicy()
	if policy.WarmupSamples != 1 || policy.RetainedSamples != 10 || policy.DiagnosticSamples != 1 {
		t.Fatalf("sampling policy = %+v", policy)
	}
}

func TestArtifactRejectsInvalidRuns(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ArtifactBundle)
	}{
		{name: "partial sample", mutate: func(b *ArtifactBundle) { b.Result.Samples[0].LoadMs = 0 }},
		{name: "duplicate identity", mutate: func(b *ArtifactBundle) { b.Result.Samples[1].ID = b.Result.Samples[0].ID }},
		{name: "non-finite timing", mutate: func(b *ArtifactBundle) { b.Result.Samples[0].DisplayReadyMs = math.NaN() }},
		{name: "missing state", mutate: func(b *ArtifactBundle) { b.Result.Metadata.State = StateBoundary{} }},
		{name: "fixture dimensions", mutate: func(b *ArtifactBundle) { b.Result.Samples[0].NaturalWidth++ }},
		{name: "traced retained row", mutate: func(b *ArtifactBundle) { b.Result.Samples[0].Traced = true }},
		{name: "untraced diagnostic", mutate: func(b *ArtifactBundle) { b.Diagnostic.Sample.Traced = false }},
		{name: "missing runtime trace", mutate: func(b *ArtifactBundle) { b.RuntimeTrace = nil }},
		{name: "missing runtime trace name", mutate: func(b *ArtifactBundle) { b.Diagnostic.RuntimeTraceFile = "" }},
		{
			name: "cross-engine CPU profile",
			mutate: func(b *ArtifactBundle) {
				b.Result.Metadata.Engine = "firefox"
				b.Diagnostic.Engine = "firefox"
				b.Diagnostic.BrowserCPUProfileFile = artifactBrowserCPUProfileFile
				b.BrowserCPUProfile = []byte("profile")
			},
		},
		{
			name: "missing CPU profile",
			mutate: func(b *ArtifactBundle) {
				b.Diagnostic.BrowserCPUProfileFile = artifactBrowserCPUProfileFile
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := validArtifactBundle(t)
			test.mutate(&bundle)
			if err := bundle.Validate(); err == nil {
				t.Fatal("invalid artifact validated")
			}
		})
	}
}

func TestArtifactPublicationIsAtomic(t *testing.T) {
	root := t.TempDir()
	publisher, err := NewArtifactPublisher(root)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reject incomplete input before its final engine directory appears.
	invalid := validArtifactBundle(t)
	invalid.Result.Samples = invalid.Result.Samples[:RetainedSampleCount-1]
	if _, err := publisher.Publish(invalid); err == nil {
		t.Fatal("incomplete artifact published")
	}
	finalDir := filepath.Join(root, invalid.Result.Metadata.RunID, invalid.Result.Metadata.Engine)
	if _, err := os.Stat(finalDir); !os.IsNotExist(err) {
		t.Fatalf("partial artifact became visible: %v", err)
	}

	// Reject a second publication without changing the complete first result.
	bundle := validArtifactBundle(t)
	artifactDir, err := publisher.Publish(bundle)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := publisher.Publish(bundle); err == nil {
		t.Fatal("duplicate artifact publication succeeded")
	}
	if _, err := ReadArtifact(artifactDir); err != nil {
		t.Fatalf("first artifact changed after duplicate publication: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(artifactDir))
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(entries) != 1 || entries[0].Name() != bundle.Result.Metadata.Engine {
		t.Fatalf("run directory contains partial entries: %v", entries)
	}
}

func TestReadArtifactRejectsCorruption(t *testing.T) {
	for _, test := range []struct {
		name       string
		file       string
		cpuProfile bool
	}{
		{name: "result", file: artifactResultFile},
		{name: "runtime trace", file: artifactRuntimeTraceFile},
		{name: "browser CPU profile", file: artifactBrowserCPUProfileFile, cpuProfile: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher, err := NewArtifactPublisher(t.TempDir())
			if err != nil {
				t.Fatal(err.Error())
			}
			bundle := validArtifactBundle(t)
			if test.cpuProfile {
				bundle.Diagnostic.BrowserCPUProfileFile = artifactBrowserCPUProfileFile
				bundle.BrowserCPUProfile = []byte("deterministic CPU profile")
			}
			artifactDir, err := publisher.Publish(bundle)
			if err != nil {
				t.Fatal(err.Error())
			}
			path := filepath.Join(artifactDir, test.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err.Error())
			}
			data[len(data)/2] ^= 1
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err.Error())
			}
			if _, err := ReadArtifact(artifactDir); err == nil {
				t.Fatal("corrupted artifact validated")
			}
		})
	}
}

func TestReadArtifactRejectsUnmanifestedFile(t *testing.T) {
	publisher, err := NewArtifactPublisher(t.TempDir())
	if err != nil {
		t.Fatal(err.Error())
	}
	artifactDir, err := publisher.Publish(validArtifactBundle(t))
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "extra.data"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := ReadArtifact(artifactDir); err == nil {
		t.Fatal("artifact with an unmanifested file validated")
	}
}

func validArtifactBundle(t *testing.T) ArtifactBundle {
	t.Helper()
	metadata := testRunMetadata()
	samples := make([]Sample, len(retainedTestValues))
	for idx, value := range retainedTestValues {
		samples[idx] = testSample("retained-"+strconv.Itoa(idx+1), value, false)
	}
	summary, err := SummarizeSamples(samples)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ArtifactBundle{
		Result: Artifact{
			SchemaVersion: artifactSchemaVersion,
			Metadata:      metadata,
			Sampling:      fixedSamplingPolicy(),
			Warmup:        testSample("warmup-1", 11, false),
			Samples:       samples,
			Summary:       summary,
		},
		Diagnostic: DiagnosticArtifact{
			SchemaVersion:    artifactSchemaVersion,
			RunID:            metadata.RunID,
			Engine:           metadata.Engine,
			Sample:           testSample("diagnostic-1", 12, true),
			RuntimeTraceFile: artifactRuntimeTraceFile,
		},
		RuntimeTrace: []byte("deterministic runtime trace"),
	}
}

func testRunMetadata() RunMetadata {
	return RunMetadata{
		RunID:                "run-20260804-1308",
		Engine:               "chromium",
		EngineVersion:        "140.0.7339.0",
		Compiler:             "goscript",
		SpacewaveRevision:    "1ab0223bb",
		GoScriptRevision:     "f40cf597bdfc",
		BuildMode:            "unbundled-diagnostic",
		WorkerMode:           "dedicated-worker",
		StorageBackend:       "opfs",
		RuntimeState:         "runtime-cold-retained-opfs",
		ProjectedURLTemplate: "/p/spacewave-core/fs/u/{world}/so/{space}/-/files/-/fixture.png?sample={id}",
		Fixture: Fixture{
			Generator:          "deterministic-test-fixture",
			GeneratorRevision:  "1",
			Encoder:            "test-encoder",
			EncoderEnvironment: "go-test",
			SHA256:             strings.Repeat("a", 64),
			EncodedBytes:       4_194_304,
			Width:              1024,
			Height:             1024,
			ColorModel:         "RGBA",
			Path:               "fixture.png",
		},
		State: StateBoundary{
			Retained:  []string{"browser-process", "browser-context", "opfs-files"},
			Recreated: []string{"page", "dedicated-worker", "go-runtime"},
		},
		UnavailableFields: []string{},
	}
}

func testSample(id string, displayReadyMs float64, traced bool) Sample {
	return Sample{
		ID:              id,
		RequestStartMs:  0,
		ResponseStartMs: displayReadyMs * 0.2,
		ResponseEndMs:   displayReadyMs * 0.5,
		LoadMs:          displayReadyMs * 0.7,
		DecodeMs:        displayReadyMs * 0.9,
		FrameMs:         displayReadyMs,
		DisplayReadyMs:  displayReadyMs,
		NaturalWidth:    1024,
		NaturalHeight:   1024,
		TransferSize:    4_194_304,
		DecodedBodySize: 4_194_304,
		Traced:          traced,
	}
}

func testSampleRequests() []SampleRequest {
	requests := make([]SampleRequest, 0, WarmupSampleCount+RetainedSampleCount+DiagnosticSampleCount)
	requests = append(requests, SampleRequest{Kind: SampleKindWarmup, Number: 1})
	for number := 1; number <= RetainedSampleCount; number++ {
		requests = append(requests, SampleRequest{Kind: SampleKindRetained, Number: number})
	}
	requests = append(requests, SampleRequest{Kind: SampleKindDiagnostic, Number: 1, Trace: true})
	return requests
}

func sampleRequestName(request SampleRequest) string {
	return string(request.Kind) + "-" + strconv.Itoa(request.Number)
}

// _ is a type assertion
var _ Workload = (*deterministicWorkload)(nil)
