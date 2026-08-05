//go:build !js

package goscriptbench

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

type artifactManifest struct {
	SchemaVersion           int
	ResultFile              string
	ResultSHA256            string
	DiagnosticFile          string
	DiagnosticSHA256        string
	RuntimeTraceFile        string
	RuntimeTraceSHA256      string
	BrowserCPUProfileFile   string
	BrowserCPUProfileSHA256 string
}

func marshalArtifactData(artifact Artifact) []byte {
	var arena fastjson.Arena
	value := arena.NewObject()
	value.Set("schemaVersion", arena.NewNumberInt(artifact.SchemaVersion))
	value.Set("metadata", marshalRunMetadataValue(&arena, artifact.Metadata))
	value.Set("sampling", marshalSamplingPolicyValue(&arena, artifact.Sampling))
	value.Set("warmup", marshalSampleValue(&arena, artifact.Warmup))
	value.Set("samples", marshalSamplesValue(&arena, artifact.Samples))
	value.Set("summary", marshalSummaryValue(&arena, artifact.Summary))
	return append(value.MarshalTo(nil), '\n')
}

func marshalDiagnosticData(diagnostic DiagnosticArtifact) []byte {
	var arena fastjson.Arena
	value := arena.NewObject()
	value.Set("schemaVersion", arena.NewNumberInt(diagnostic.SchemaVersion))
	value.Set("runId", arena.NewString(diagnostic.RunID))
	value.Set("engine", arena.NewString(diagnostic.Engine))
	value.Set("sample", marshalSampleValue(&arena, diagnostic.Sample))
	value.Set("runtimeTraceFile", arena.NewString(diagnostic.RuntimeTraceFile))
	value.Set("browserCpuProfileFile", arena.NewString(diagnostic.BrowserCPUProfileFile))
	return append(value.MarshalTo(nil), '\n')
}

func marshalManifestData(manifest artifactManifest) []byte {
	var arena fastjson.Arena
	value := arena.NewObject()
	value.Set("schemaVersion", arena.NewNumberInt(manifest.SchemaVersion))
	value.Set("resultFile", arena.NewString(manifest.ResultFile))
	value.Set("resultSha256", arena.NewString(manifest.ResultSHA256))
	value.Set("diagnosticFile", arena.NewString(manifest.DiagnosticFile))
	value.Set("diagnosticSha256", arena.NewString(manifest.DiagnosticSHA256))
	value.Set("runtimeTraceFile", arena.NewString(manifest.RuntimeTraceFile))
	value.Set("runtimeTraceSha256", arena.NewString(manifest.RuntimeTraceSHA256))
	value.Set("browserCpuProfileFile", arena.NewString(manifest.BrowserCPUProfileFile))
	value.Set("browserCpuProfileSha256", arena.NewString(manifest.BrowserCPUProfileSHA256))
	return append(value.MarshalTo(nil), '\n')
}

func marshalRunMetadataValue(arena *fastjson.Arena, metadata RunMetadata) *fastjson.Value {
	value := arena.NewObject()
	value.Set("runId", arena.NewString(metadata.RunID))
	value.Set("engine", arena.NewString(metadata.Engine))
	value.Set("engineVersion", arena.NewString(metadata.EngineVersion))
	value.Set("compiler", arena.NewString(metadata.Compiler))
	value.Set("spacewaveRevision", arena.NewString(metadata.SpacewaveRevision))
	value.Set("goScriptRevision", arena.NewString(metadata.GoScriptRevision))
	value.Set("buildMode", arena.NewString(metadata.BuildMode))
	value.Set("workerMode", arena.NewString(metadata.WorkerMode))
	value.Set("storageBackend", arena.NewString(metadata.StorageBackend))
	value.Set("runtimeState", arena.NewString(metadata.RuntimeState))
	value.Set("projectedUrlTemplate", arena.NewString(metadata.ProjectedURLTemplate))
	value.Set("fixture", marshalFixtureValue(arena, metadata.Fixture))
	value.Set("state", marshalStateBoundaryValue(arena, metadata.State))
	value.Set("unavailableFields", marshalStringSliceValue(arena, metadata.UnavailableFields))
	return value
}

func marshalFixtureValue(arena *fastjson.Arena, fixture Fixture) *fastjson.Value {
	value := arena.NewObject()
	value.Set("generator", arena.NewString(fixture.Generator))
	value.Set("generatorRevision", arena.NewString(fixture.GeneratorRevision))
	value.Set("encoder", arena.NewString(fixture.Encoder))
	value.Set("encoderEnvironment", arena.NewString(fixture.EncoderEnvironment))
	value.Set("sha256", arena.NewString(fixture.SHA256))
	value.Set("encodedBytes", arena.NewNumberString(strconv.FormatInt(fixture.EncodedBytes, 10)))
	value.Set("width", arena.NewNumberInt(fixture.Width))
	value.Set("height", arena.NewNumberInt(fixture.Height))
	value.Set("colorModel", arena.NewString(fixture.ColorModel))
	value.Set("path", arena.NewString(fixture.Path))
	return value
}

func marshalStateBoundaryValue(arena *fastjson.Arena, state StateBoundary) *fastjson.Value {
	value := arena.NewObject()
	value.Set("retained", marshalStringSliceValue(arena, state.Retained))
	value.Set("recreated", marshalStringSliceValue(arena, state.Recreated))
	return value
}

func marshalSamplingPolicyValue(arena *fastjson.Arena, sampling SamplingPolicy) *fastjson.Value {
	value := arena.NewObject()
	value.Set("warmupSamples", arena.NewNumberInt(sampling.WarmupSamples))
	value.Set("retainedSamples", arena.NewNumberInt(sampling.RetainedSamples))
	value.Set("diagnosticSamples", arena.NewNumberInt(sampling.DiagnosticSamples))
	value.Set("summaryMethod", arena.NewString(sampling.SummaryMethod))
	return value
}

func marshalSamplesValue(arena *fastjson.Arena, samples []Sample) *fastjson.Value {
	value := arena.NewArray()
	for idx, sample := range samples {
		value.SetArrayItem(idx, marshalSampleValue(arena, sample))
	}
	return value
}

func marshalSampleValue(arena *fastjson.Arena, sample Sample) *fastjson.Value {
	value := arena.NewObject()
	value.Set("id", arena.NewString(sample.ID))
	value.Set("requestStartMs", arena.NewNumberFloat64(sample.RequestStartMs))
	value.Set("responseStartMs", arena.NewNumberFloat64(sample.ResponseStartMs))
	value.Set("responseEndMs", arena.NewNumberFloat64(sample.ResponseEndMs))
	value.Set("loadMs", arena.NewNumberFloat64(sample.LoadMs))
	value.Set("decodeMs", arena.NewNumberFloat64(sample.DecodeMs))
	value.Set("frameMs", arena.NewNumberFloat64(sample.FrameMs))
	value.Set("displayReadyMs", arena.NewNumberFloat64(sample.DisplayReadyMs))
	value.Set("naturalWidth", arena.NewNumberInt(sample.NaturalWidth))
	value.Set("naturalHeight", arena.NewNumberInt(sample.NaturalHeight))
	value.Set("transferSize", arena.NewNumberString(strconv.FormatInt(sample.TransferSize, 10)))
	value.Set("decodedBodySize", arena.NewNumberString(strconv.FormatInt(sample.DecodedBodySize, 10)))
	traced := arena.NewFalse()
	if sample.Traced {
		traced = arena.NewTrue()
	}
	value.Set("traced", traced)
	return value
}

func marshalSummaryValue(arena *fastjson.Arena, summary Summary) *fastjson.Value {
	value := arena.NewObject()
	value.Set("method", arena.NewString(summary.Method))
	value.Set("sampleCount", arena.NewNumberInt(summary.SampleCount))
	value.Set("medianDisplayReadyMs", arena.NewNumberFloat64(summary.MedianDisplayReadyMs))
	value.Set("p95DisplayReadyMs", arena.NewNumberFloat64(summary.P95DisplayReadyMs))
	return value
}

func marshalStringSliceValue(arena *fastjson.Arena, values []string) *fastjson.Value {
	value := arena.NewArray()
	for idx, item := range values {
		value.SetArrayItem(idx, arena.NewString(item))
	}
	return value
}

func parseArtifactData(data []byte) (Artifact, error) {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return Artifact{}, errors.Wrap(err, "parse result JSON")
	}
	if value.Type() != fastjson.TypeObject {
		return Artifact{}, errors.New("result JSON root must be an object")
	}
	artifact := Artifact{}
	if artifact.SchemaVersion, err = parseInt(value, "schemaVersion"); err != nil {
		return Artifact{}, err
	}
	if artifact.Metadata, err = parseRunMetadata(value.Get("metadata")); err != nil {
		return Artifact{}, errors.Wrap(err, "parse metadata")
	}
	if artifact.Sampling, err = parseSamplingPolicy(value.Get("sampling")); err != nil {
		return Artifact{}, errors.Wrap(err, "parse sampling")
	}
	if artifact.Warmup, err = parseSample(value.Get("warmup")); err != nil {
		return Artifact{}, errors.Wrap(err, "parse warm-up")
	}
	if artifact.Samples, err = parseSamples(value.Get("samples")); err != nil {
		return Artifact{}, errors.Wrap(err, "parse retained samples")
	}
	if artifact.Summary, err = parseSummary(value.Get("summary")); err != nil {
		return Artifact{}, errors.Wrap(err, "parse summary")
	}
	return artifact, nil
}

func parseDiagnosticData(data []byte) (DiagnosticArtifact, error) {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return DiagnosticArtifact{}, errors.Wrap(err, "parse diagnostic JSON")
	}
	if value.Type() != fastjson.TypeObject {
		return DiagnosticArtifact{}, errors.New("diagnostic JSON root must be an object")
	}
	diagnostic := DiagnosticArtifact{}
	if diagnostic.SchemaVersion, err = parseInt(value, "schemaVersion"); err != nil {
		return DiagnosticArtifact{}, err
	}
	if diagnostic.RunID, err = parseString(value, "runId"); err != nil {
		return DiagnosticArtifact{}, err
	}
	if diagnostic.Engine, err = parseString(value, "engine"); err != nil {
		return DiagnosticArtifact{}, err
	}
	if diagnostic.Sample, err = parseSample(value.Get("sample")); err != nil {
		return DiagnosticArtifact{}, errors.Wrap(err, "parse diagnostic sample")
	}
	if diagnostic.RuntimeTraceFile, err = parseString(value, "runtimeTraceFile"); err != nil {
		return DiagnosticArtifact{}, err
	}
	if diagnostic.BrowserCPUProfileFile, err = parseString(value, "browserCpuProfileFile"); err != nil {
		return DiagnosticArtifact{}, err
	}
	return diagnostic, nil
}

func parseManifestData(data []byte) (artifactManifest, error) {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return artifactManifest{}, errors.Wrap(err, "parse artifact manifest")
	}
	if value.Type() != fastjson.TypeObject {
		return artifactManifest{}, errors.New("artifact manifest root must be an object")
	}
	manifest := artifactManifest{}
	if manifest.SchemaVersion, err = parseInt(value, "schemaVersion"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.ResultFile, err = parseString(value, "resultFile"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.ResultSHA256, err = parseString(value, "resultSha256"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.DiagnosticFile, err = parseString(value, "diagnosticFile"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.DiagnosticSHA256, err = parseString(value, "diagnosticSha256"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.RuntimeTraceFile, err = parseString(value, "runtimeTraceFile"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.RuntimeTraceSHA256, err = parseString(value, "runtimeTraceSha256"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.BrowserCPUProfileFile, err = parseString(value, "browserCpuProfileFile"); err != nil {
		return artifactManifest{}, err
	}
	if manifest.BrowserCPUProfileSHA256, err = parseString(value, "browserCpuProfileSha256"); err != nil {
		return artifactManifest{}, err
	}
	return manifest, nil
}

func parseRunMetadata(value *fastjson.Value) (RunMetadata, error) {
	if value == nil || value.Type() != fastjson.TypeObject {
		return RunMetadata{}, errors.New("metadata must be an object")
	}
	metadata := RunMetadata{}
	var err error
	if metadata.RunID, err = parseString(value, "runId"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.Engine, err = parseString(value, "engine"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.EngineVersion, err = parseString(value, "engineVersion"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.Compiler, err = parseString(value, "compiler"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.SpacewaveRevision, err = parseString(value, "spacewaveRevision"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.GoScriptRevision, err = parseString(value, "goScriptRevision"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.BuildMode, err = parseString(value, "buildMode"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.WorkerMode, err = parseString(value, "workerMode"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.StorageBackend, err = parseString(value, "storageBackend"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.RuntimeState, err = parseString(value, "runtimeState"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.ProjectedURLTemplate, err = parseString(value, "projectedUrlTemplate"); err != nil {
		return RunMetadata{}, err
	}
	if metadata.Fixture, err = parseFixture(value.Get("fixture")); err != nil {
		return RunMetadata{}, errors.Wrap(err, "parse fixture")
	}
	if metadata.State, err = parseStateBoundary(value.Get("state")); err != nil {
		return RunMetadata{}, errors.Wrap(err, "parse state")
	}
	if metadata.UnavailableFields, err = parseStringSlice(value, "unavailableFields"); err != nil {
		return RunMetadata{}, err
	}
	return metadata, nil
}

func parseFixture(value *fastjson.Value) (Fixture, error) {
	if value == nil || value.Type() != fastjson.TypeObject {
		return Fixture{}, errors.New("fixture must be an object")
	}
	fixture := Fixture{}
	var err error
	if fixture.Generator, err = parseString(value, "generator"); err != nil {
		return Fixture{}, err
	}
	if fixture.GeneratorRevision, err = parseString(value, "generatorRevision"); err != nil {
		return Fixture{}, err
	}
	if fixture.Encoder, err = parseString(value, "encoder"); err != nil {
		return Fixture{}, err
	}
	if fixture.EncoderEnvironment, err = parseString(value, "encoderEnvironment"); err != nil {
		return Fixture{}, err
	}
	if fixture.SHA256, err = parseString(value, "sha256"); err != nil {
		return Fixture{}, err
	}
	if fixture.EncodedBytes, err = parseInt64(value, "encodedBytes"); err != nil {
		return Fixture{}, err
	}
	if fixture.Width, err = parseInt(value, "width"); err != nil {
		return Fixture{}, err
	}
	if fixture.Height, err = parseInt(value, "height"); err != nil {
		return Fixture{}, err
	}
	if fixture.ColorModel, err = parseString(value, "colorModel"); err != nil {
		return Fixture{}, err
	}
	if fixture.Path, err = parseString(value, "path"); err != nil {
		return Fixture{}, err
	}
	return fixture, nil
}

func parseStateBoundary(value *fastjson.Value) (StateBoundary, error) {
	if value == nil || value.Type() != fastjson.TypeObject {
		return StateBoundary{}, errors.New("state boundary must be an object")
	}
	retained, err := parseStringSlice(value, "retained")
	if err != nil {
		return StateBoundary{}, err
	}
	recreated, err := parseStringSlice(value, "recreated")
	if err != nil {
		return StateBoundary{}, err
	}
	return StateBoundary{Retained: retained, Recreated: recreated}, nil
}

func parseSamplingPolicy(value *fastjson.Value) (SamplingPolicy, error) {
	if value == nil || value.Type() != fastjson.TypeObject {
		return SamplingPolicy{}, errors.New("sampling policy must be an object")
	}
	policy := SamplingPolicy{}
	var err error
	if policy.WarmupSamples, err = parseInt(value, "warmupSamples"); err != nil {
		return SamplingPolicy{}, err
	}
	if policy.RetainedSamples, err = parseInt(value, "retainedSamples"); err != nil {
		return SamplingPolicy{}, err
	}
	if policy.DiagnosticSamples, err = parseInt(value, "diagnosticSamples"); err != nil {
		return SamplingPolicy{}, err
	}
	if policy.SummaryMethod, err = parseString(value, "summaryMethod"); err != nil {
		return SamplingPolicy{}, err
	}
	return policy, nil
}

func parseSamples(value *fastjson.Value) ([]Sample, error) {
	if value == nil || value.Type() != fastjson.TypeArray {
		return nil, errors.New("samples must be an array")
	}
	values, err := value.Array()
	if err != nil {
		return nil, err
	}
	samples := make([]Sample, len(values))
	for idx, item := range values {
		if samples[idx], err = parseSample(item); err != nil {
			return nil, errors.Wrapf(err, "parse sample %d", idx+1)
		}
	}
	return samples, nil
}

func parseSample(value *fastjson.Value) (Sample, error) {
	if value == nil || value.Type() != fastjson.TypeObject {
		return Sample{}, errors.New("sample must be an object")
	}
	sample := Sample{}
	var err error
	if sample.ID, err = parseString(value, "id"); err != nil {
		return Sample{}, err
	}
	if sample.RequestStartMs, err = parseFloat64(value, "requestStartMs"); err != nil {
		return Sample{}, err
	}
	if sample.ResponseStartMs, err = parseFloat64(value, "responseStartMs"); err != nil {
		return Sample{}, err
	}
	if sample.ResponseEndMs, err = parseFloat64(value, "responseEndMs"); err != nil {
		return Sample{}, err
	}
	if sample.LoadMs, err = parseFloat64(value, "loadMs"); err != nil {
		return Sample{}, err
	}
	if sample.DecodeMs, err = parseFloat64(value, "decodeMs"); err != nil {
		return Sample{}, err
	}
	if sample.FrameMs, err = parseFloat64(value, "frameMs"); err != nil {
		return Sample{}, err
	}
	if sample.DisplayReadyMs, err = parseFloat64(value, "displayReadyMs"); err != nil {
		return Sample{}, err
	}
	if sample.NaturalWidth, err = parseInt(value, "naturalWidth"); err != nil {
		return Sample{}, err
	}
	if sample.NaturalHeight, err = parseInt(value, "naturalHeight"); err != nil {
		return Sample{}, err
	}
	if sample.TransferSize, err = parseInt64(value, "transferSize"); err != nil {
		return Sample{}, err
	}
	if sample.DecodedBodySize, err = parseInt64(value, "decodedBodySize"); err != nil {
		return Sample{}, err
	}
	if sample.Traced, err = parseBool(value, "traced"); err != nil {
		return Sample{}, err
	}
	return sample, nil
}

func parseSummary(value *fastjson.Value) (Summary, error) {
	if value == nil || value.Type() != fastjson.TypeObject {
		return Summary{}, errors.New("summary must be an object")
	}
	summary := Summary{}
	var err error
	if summary.Method, err = parseString(value, "method"); err != nil {
		return Summary{}, err
	}
	if summary.SampleCount, err = parseInt(value, "sampleCount"); err != nil {
		return Summary{}, err
	}
	if summary.MedianDisplayReadyMs, err = parseFloat64(value, "medianDisplayReadyMs"); err != nil {
		return Summary{}, err
	}
	if summary.P95DisplayReadyMs, err = parseFloat64(value, "p95DisplayReadyMs"); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func parseString(value *fastjson.Value, field string) (string, error) {
	item := value.Get(field)
	if item == nil || item.Type() != fastjson.TypeString {
		return "", errors.Errorf("field %q must be a string", field)
	}
	data, err := item.StringBytes()
	if err != nil {
		return "", errors.Wrapf(err, "read string field %q", field)
	}
	return string(data), nil
}

func parseInt(value *fastjson.Value, field string) (int, error) {
	item := value.Get(field)
	if item == nil || item.Type() != fastjson.TypeNumber {
		return 0, errors.Errorf("field %q must be a number", field)
	}
	number, err := item.Int()
	if err != nil {
		return 0, errors.Wrapf(err, "read integer field %q", field)
	}
	return number, nil
}

func parseInt64(value *fastjson.Value, field string) (int64, error) {
	item := value.Get(field)
	if item == nil || item.Type() != fastjson.TypeNumber {
		return 0, errors.Errorf("field %q must be a number", field)
	}
	number, err := item.Int64()
	if err != nil {
		return 0, errors.Wrapf(err, "read integer field %q", field)
	}
	return number, nil
}

func parseFloat64(value *fastjson.Value, field string) (float64, error) {
	item := value.Get(field)
	if item == nil || item.Type() != fastjson.TypeNumber {
		return 0, errors.Errorf("field %q must be a number", field)
	}
	number, err := item.Float64()
	if err != nil {
		return 0, errors.Wrapf(err, "read number field %q", field)
	}
	return number, nil
}

func parseBool(value *fastjson.Value, field string) (bool, error) {
	item := value.Get(field)
	if item == nil || (item.Type() != fastjson.TypeTrue && item.Type() != fastjson.TypeFalse) {
		return false, errors.Errorf("field %q must be a boolean", field)
	}
	result, err := item.Bool()
	if err != nil {
		return false, errors.Wrapf(err, "read boolean field %q", field)
	}
	return result, nil
}

func parseStringSlice(value *fastjson.Value, field string) ([]string, error) {
	item := value.Get(field)
	if item == nil || item.Type() != fastjson.TypeArray {
		return nil, errors.Errorf("field %q must be an array", field)
	}
	values, err := item.Array()
	if err != nil {
		return nil, errors.Wrapf(err, "read array field %q", field)
	}
	result := make([]string, len(values))
	for idx, entry := range values {
		if entry.Type() != fastjson.TypeString {
			return nil, errors.Errorf("field %q contains a non-string entry", field)
		}
		data, err := entry.StringBytes()
		if err != nil {
			return nil, errors.Wrapf(err, "read array field %q entry %d", field, idx)
		}
		result[idx] = string(data)
	}
	return result, nil
}
