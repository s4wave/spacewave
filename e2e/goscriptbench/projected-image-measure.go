//go:build !js

package goscriptbench

import (
	"context"
	"math"
	"net/url"
	"strconv"

	"github.com/pkg/errors"
)

// Measure runs one scalar or diagnostic projected-image sample after Restart.
func (p *ProjectedImage) Measure(ctx context.Context, request SampleRequest) (Measurement, error) {
	if !request.Trace {
		sample, err := p.MeasureUntraced(ctx, request)
		if err != nil {
			return Measurement{}, err
		}
		return Measurement{Sample: sample}, nil
	}
	if err := ctx.Err(); err != nil {
		return Measurement{}, err
	}
	id, err := projectedImageDiagnosticSampleID(request)
	if err != nil {
		return Measurement{}, err
	}
	projectedURL, err := p.projectedImageSampleURL(id)
	if err != nil {
		return Measurement{}, err
	}

	// Capture the runtime trace and optional Chromium profile around one browser action.
	var sample Sample
	var browserCPUProfile []byte
	runtimeTrace, err := p.session.CaptureTrace(ctx, "goscriptbench-"+id, func(traceCtx context.Context) error {
		measured, profile, err := p.captureBrowserCPUProfile(
			traceCtx,
			func(measureCtx context.Context) (Sample, error) {
				return p.measureProjectedImageURL(
					measureCtx,
					id,
					projectedURL,
					p.metadata.Fixture.Width,
					p.metadata.Fixture.Height,
				)
			},
		)
		if err != nil {
			return err
		}
		sample = measured
		browserCPUProfile = profile
		return nil
	})
	if err != nil {
		return Measurement{}, errors.Wrap(err, "capture projected-image diagnostic")
	}
	sample.Traced = true
	return Measurement{
		Sample:            sample,
		RuntimeTrace:      runtimeTrace,
		BrowserCPUProfile: browserCPUProfile,
	}, nil
}

// Validate checks one scalar or diagnostic sample against its request and fixture.
func (p *ProjectedImage) Validate(
	ctx context.Context,
	request SampleRequest,
	sample Sample,
) error {
	if !request.Trace {
		return p.ValidateUntraced(ctx, request, sample)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := projectedImageDiagnosticSampleID(request)
	if err != nil {
		return err
	}
	if sample.ID != id {
		return errors.Errorf("projected-image sample ID %q differs from request %q", sample.ID, id)
	}
	if !sample.Traced {
		return errors.Errorf("projected-image diagnostic sample %q omits tracing", sample.ID)
	}
	return sample.Validate(p.metadata)
}

// MeasureUntraced runs one scalar projected-image sample after Restart.
func (p *ProjectedImage) MeasureUntraced(ctx context.Context, request SampleRequest) (Sample, error) {
	if err := ctx.Err(); err != nil {
		return Sample{}, err
	}
	id, err := projectedImageUntracedSampleID(request)
	if err != nil {
		return Sample{}, err
	}
	projectedURL, err := p.projectedImageSampleURL(id)
	if err != nil {
		return Sample{}, err
	}
	return p.measureProjectedImageURL(
		ctx,
		id,
		projectedURL,
		p.metadata.Fixture.Width,
		p.metadata.Fixture.Height,
	)
}

// ValidateUntraced checks one scalar sample against its request and fixture.
func (p *ProjectedImage) ValidateUntraced(
	ctx context.Context,
	request SampleRequest,
	sample Sample,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, err := projectedImageUntracedSampleID(request)
	if err != nil {
		return err
	}
	if sample.ID != id {
		return errors.Errorf("projected-image sample ID %q differs from request %q", sample.ID, id)
	}
	if sample.Traced {
		return errors.Errorf("projected-image scalar sample %q reports tracing", sample.ID)
	}
	return sample.Validate(p.metadata)
}

func (p *ProjectedImage) projectedImageSampleURL(id string) (string, error) {
	if !p.readyToMeasure {
		return "", errors.New("projected-image sample requires a completed runtime restart")
	}
	token := p.config.RunID + "-" + id
	if _, measured := p.measuredSamples[token]; measured {
		return "", errors.Errorf("projected-image sample %q was already measured", id)
	}

	// Consume the restarted runtime and cache token before starting the browser action.
	p.readyToMeasure = false
	p.measuredSamples[token] = struct{}{}
	return p.harness.BaseURL() +
		projectedImageURL(p.sessionIndex, p.spaceID) +
		"&sample=" + url.QueryEscape(token), nil
}

func projectedImageDiagnosticSampleID(request SampleRequest) (string, error) {
	if !request.Trace {
		return "", errors.New("projected-image diagnostic sample requires tracing")
	}
	if request.Kind != SampleKindDiagnostic || request.Number != 1 {
		return "", errors.New("projected-image diagnostic request must be sample one")
	}
	return "diagnostic-1", nil
}

func projectedImageUntracedSampleID(request SampleRequest) (string, error) {
	if request.Trace {
		return "", errors.New("projected-image scalar samples cannot enable tracing")
	}
	switch request.Kind {
	case SampleKindWarmup:
		if request.Number != 1 {
			return "", errors.New("projected-image warm-up sample number must be one")
		}
	case SampleKindRetained:
		if request.Number < 1 || request.Number > RetainedSampleCount {
			return "", errors.Errorf("projected-image retained sample number must be between 1 and %d", RetainedSampleCount)
		}
	case SampleKindDiagnostic:
		return "", errors.New("projected-image diagnostic samples require trace capture")
	default:
		return "", errors.Errorf("projected-image sample kind %q is unknown", request.Kind)
	}
	return string(request.Kind) + "-" + strconv.Itoa(request.Number), nil
}

func (p *ProjectedImage) measureProjectedImageURL(
	ctx context.Context,
	id string,
	projectedURL string,
	expectedWidth int,
	expectedHeight int,
) (Sample, error) {
	if err := ctx.Err(); err != nil {
		return Sample{}, err
	}
	if p.session == nil {
		return Sample{}, errors.New("projected-image workload is not set up")
	}
	if id == "" || projectedURL == "" || expectedWidth <= 0 || expectedHeight <= 0 {
		return Sample{}, errors.New("projected-image measurement identity is incomplete")
	}

	// Run the controlled image action entirely on the browser clock.
	raw, err := p.session.Page().Evaluate(p.harness.Script(projectedImageMeasureScript), map[string]any{
		"id":             id,
		"projectedUrl":   projectedURL,
		"expectedWidth":  expectedWidth,
		"expectedHeight": expectedHeight,
		"deadlineMs":     120000,
	})
	if err != nil {
		return Sample{}, errors.Wrap(err, "measure projected image")
	}

	// Decode and verify the browser result without reconstructing its timing relationships.
	result, err := projectedImageSampleFromBrowser(raw)
	if err != nil {
		return Sample{}, err
	}
	if err := result.validateRequest(id, projectedURL); err != nil {
		return Sample{}, err
	}
	if err := ctx.Err(); err != nil {
		return Sample{}, err
	}
	return result.sample, nil
}

type projectedImageBrowserSample struct {
	sample             Sample
	projectedURL       string
	resourceEntryCount int
}

func (s projectedImageBrowserSample) validateRequest(id, projectedURL string) error {
	if s.sample.ID != id || s.projectedURL != projectedURL {
		return errors.New("projected-image browser result differs from its request")
	}
	if s.resourceEntryCount != 1 {
		return errors.Errorf("projected-image resource entry count = %d, want 1", s.resourceEntryCount)
	}
	return nil
}

func projectedImageSampleFromBrowser(raw any) (projectedImageBrowserSample, error) {
	result, ok := raw.(map[string]any)
	if !ok {
		return projectedImageBrowserSample{}, errors.Errorf("unexpected projected-image sample %T", raw)
	}

	var sample projectedImageBrowserSample
	var err error
	if sample.projectedURL, err = projectedImageBrowserString(result, "projectedUrl"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.resourceEntryCount, err = projectedImageBrowserInt(result, "resourceEntryCount"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.ID, err = projectedImageBrowserString(result, "id"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.RequestStartMs, err = projectedImageBrowserNumber(result, "requestStartMs"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.ResponseStartMs, err = projectedImageBrowserNumber(result, "responseStartMs"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.ResponseEndMs, err = projectedImageBrowserNumber(result, "responseEndMs"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.LoadMs, err = projectedImageBrowserNumber(result, "loadMs"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.DecodeMs, err = projectedImageBrowserNumber(result, "decodeMs"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.FrameMs, err = projectedImageBrowserNumber(result, "frameMs"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.DisplayReadyMs, err = projectedImageBrowserNumber(result, "displayReadyMs"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.NaturalWidth, err = projectedImageBrowserInt(result, "naturalWidth"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.NaturalHeight, err = projectedImageBrowserInt(result, "naturalHeight"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.TransferSize, err = projectedImageBrowserInt64(result, "transferSize"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.DecodedBodySize, err = projectedImageBrowserInt64(result, "decodedBodySize"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	if sample.sample.Traced, err = projectedImageBrowserBool(result, "traced"); err != nil {
		return projectedImageBrowserSample{}, err
	}
	return sample, nil
}

func projectedImageBrowserString(result map[string]any, field string) (string, error) {
	value, ok := result[field].(string)
	if !ok {
		return "", errors.Errorf("projected-image sample field %q is not a string", field)
	}
	return value, nil
}

func projectedImageBrowserNumber(result map[string]any, field string) (float64, error) {
	switch value := result[field].(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	default:
		return 0, errors.Errorf("projected-image sample field %q is not numeric", field)
	}
}

func projectedImageBrowserInt(result map[string]any, field string) (int, error) {
	number, err := projectedImageBrowserNumber(result, field)
	if err != nil {
		return 0, err
	}
	value := int(number)
	if math.Trunc(number) != number || float64(value) != number {
		return 0, errors.Errorf("projected-image sample field %q is not an integer", field)
	}
	return value, nil
}

func projectedImageBrowserInt64(result map[string]any, field string) (int64, error) {
	number, err := projectedImageBrowserNumber(result, field)
	if err != nil {
		return 0, err
	}
	value := int64(number)
	if math.Trunc(number) != number || float64(value) != number {
		return 0, errors.Errorf("projected-image sample field %q is not an integer", field)
	}
	return value, nil
}

func projectedImageBrowserBool(result map[string]any, field string) (bool, error) {
	value, ok := result[field].(bool)
	if !ok {
		return false, errors.Errorf("projected-image sample field %q is not a boolean", field)
	}
	return value, nil
}

var _ Workload = (*ProjectedImage)(nil)
