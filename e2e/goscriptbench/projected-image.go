//go:build !js

package goscriptbench

import (
	"context"
	"encoding/base64"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	wasm "github.com/s4wave/spacewave/e2e/wasm"
)

const (
	projectedImageMeasureScript = "projected-image-measure.ts"
	projectedImageSetupScript   = "projected-image-setup.ts"
)

// ProjectedImage sets up and restarts the retained-OPFS image workload.
type ProjectedImage struct {
	// t receives failures from reused browser harness helpers
	t testing.TB
	// harness provides the browser and GoScript runtime lifecycle
	harness *wasm.Harness
	// config identifies the run sources and browser engine
	config ProjectedImageConfig

	// session holds the retained BrowserContext and replaceable page
	session *wasm.TestSession
	// sessionIndex identifies the mounted browser session
	sessionIndex uint32
	// spaceID identifies the Drive containing the fixture
	spaceID string
	// metadata is complete after Setup verifies the stored fixture
	metadata RunMetadata
	// priorWorkers identifies the runtime generation replaced by Restart
	priorWorkers []playwright.Worker
	// targetHash reopens the created Drive without quickstart setup
	targetHash string
	// measuredSamples records cache tokens already consumed by browser actions
	measuredSamples map[string]struct{}
	// readyToMeasure reports that Restart completed for the next sample
	readyToMeasure bool
}

// NewProjectedImage constructs the retained-OPFS image workload.
func NewProjectedImage(t testing.TB, harness *wasm.Harness, config ProjectedImageConfig) (*ProjectedImage, error) {
	if t == nil {
		return nil, errors.New("test owner is required")
	}
	if harness == nil {
		return nil, errors.New("browser harness is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if name := harness.BrowserName(); name != config.Engine {
		return nil, errors.Errorf("configured engine %q differs from browser harness %q", config.Engine, name)
	}
	config.UnavailableFields = slices.Clone(config.UnavailableFields)
	return &ProjectedImage{
		t:               t,
		harness:         harness,
		config:          config,
		measuredSamples: make(map[string]struct{}),
	}, nil
}

// Setup generates, uploads, and reads back the fixture before sampling begins.
func (p *ProjectedImage) Setup(ctx context.Context) (RunMetadata, error) {
	p.t.Helper()
	if err := ctx.Err(); err != nil {
		return RunMetadata{}, err
	}
	if p.session != nil {
		return RunMetadata{}, errors.New("projected-image workload is already set up")
	}

	// Generate and validate the byte identity before browser storage is touched.
	data, fixture, err := GenerateProjectedImageFixture()
	if err != nil {
		return RunMetadata{}, err
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return RunMetadata{}, errors.New("locate projected-image package")
	}
	if err := p.harness.CompileScripts(filepath.Dir(sourceFile)); err != nil {
		return RunMetadata{}, errors.Wrap(err, "compile projected-image browser scripts")
	}

	// Create the retained BrowserContext and the Drive that receives the fixture.
	p.session = p.harness.NewRetainedStatePageSession(p.t)
	if _, err := p.session.Page().Evaluate(`() => {
		window.location.hash = '#/quickstart/drive'
	}`); err != nil {
		return RunMetadata{}, errors.Wrap(err, "navigate to Drive quickstart")
	}
	wasm.CompleteDriveIntroWizard(p.t, p.session.Page())
	wasm.WaitForDriveShell(p.t, p.session.Page())
	if err := p.requireDedicatedRuntime(); err != nil {
		return RunMetadata{}, err
	}
	p.sessionIndex, p.spaceID, err = projectedImageRoute(p.session.Page().URL())
	if err != nil {
		return RunMetadata{}, err
	}
	p.targetHash = "#/u/" + strconv.FormatUint(uint64(p.sessionIndex), 10) +
		"/so/" + url.PathEscape(p.spaceID)

	// Upload and read back the exact PNG before the first runtime restart.
	proof, err := p.runFixtureScript("upload", base64.StdEncoding.EncodeToString(data), fixture)
	if err != nil {
		return RunMetadata{}, err
	}
	if err := proof.validate("upload", fixture); err != nil {
		return RunMetadata{}, errors.Wrap(err, "validate uploaded fixture")
	}
	p.priorWorkers = p.session.Workers()
	if len(p.priorWorkers) == 0 {
		return RunMetadata{}, errors.New("setup observed no dedicated GoScript worker")
	}

	// Publish the verified fixture and cache boundary through workload metadata.
	browser := p.session.BrowserContext().Browser()
	if browser == nil || browser.Version() == "" {
		return RunMetadata{}, errors.New("browser version is unavailable")
	}
	p.metadata = RunMetadata{
		RunID:                p.config.RunID,
		Engine:               p.config.Engine,
		EngineVersion:        browser.Version(),
		Compiler:             "goscript",
		SpacewaveRevision:    p.config.SpacewaveRevision,
		GoScriptRevision:     p.config.GoScriptRevision,
		BuildMode:            "unbundled-diagnostic",
		WorkerMode:           "dedicated-worker",
		StorageBackend:       "opfs",
		RuntimeState:         "runtime-cold-retained-opfs",
		ProjectedURLTemplate: projectedImageURL(p.sessionIndex, p.spaceID) + "&sample={sample}",
		Fixture:              fixture,
		State: StateBoundary{
			Retained: []string{
				"browser-process",
				"browser-context",
				"service-worker-state",
				"module-cache",
				"opfs-files",
				"operating-system-page-cache",
			},
			Recreated: []string{
				"page",
				"dedicated-worker",
				"go-runtime",
				"decoded-block-cache",
				"opfs-segment-cache",
				"world-state",
				"response-cache-key",
				"decoded-image-entry",
			},
		},
		UnavailableFields: slices.Clone(p.config.UnavailableFields),
	}
	if err := p.metadata.Validate(); err != nil {
		return RunMetadata{}, err
	}
	return p.metadata, nil
}

// Restart replaces the page and dedicated runtime, then verifies the retained fixture.
func (p *ProjectedImage) Restart(ctx context.Context, _ SampleRequest) error {
	p.t.Helper()
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.session == nil || p.spaceID == "" {
		return errors.New("projected-image workload is not set up")
	}

	// Require a fresh successful restart before another browser action.
	p.readyToMeasure = false

	// Replace the document and its dedicated worker inside the retained context.
	priorPage := p.session.Page()
	if err := p.session.ReplacePageInCurrentContext(); err != nil {
		return errors.Wrap(err, "replace benchmark page")
	}
	page := p.session.Page()
	if page == nil || page == priorPage {
		return errors.New("benchmark page was not replaced")
	}
	if _, err := page.Goto(
		p.harness.BaseURL()+"/"+p.targetHash,
		playwright.PageGotoOptions{
			Timeout:   playwright.Float(240000),
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		},
	); err != nil {
		return errors.Wrap(err, "load retained Drive route")
	}
	wasm.WaitForApp(p.t, page)
	wasm.WaitForDriveShell(p.t, page)
	if err := p.requireDedicatedRuntime(); err != nil {
		return err
	}

	// Prove the new dedicated worker differs from the previous runtime generation.
	workers := p.session.Workers()
	if len(workers) == 0 {
		return errors.New("restart observed no dedicated GoScript worker")
	}
	for _, worker := range workers {
		if slices.Contains(p.priorWorkers, worker) {
			return errors.New("restart retained a prior dedicated GoScript worker")
		}
	}

	// Read the exact fixture through the restarted runtime over retained OPFS.
	proof, err := p.runFixtureScript("verify", "", p.metadata.Fixture)
	if err != nil {
		return err
	}
	if err := proof.validate("verify", p.metadata.Fixture); err != nil {
		return errors.Wrap(err, "validate retained fixture")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	p.priorWorkers = workers
	p.readyToMeasure = true
	return nil
}

func (p *ProjectedImage) runFixtureScript(action, fixtureBase64 string, fixture Fixture) (projectedImageFixtureProof, error) {
	// Execute the upload or verification module against the current page.
	raw, err := p.session.Page().Evaluate(p.harness.Script(projectedImageSetupScript), map[string]any{
		"action":        action,
		"sessionIndex":  p.sessionIndex,
		"spaceId":       p.spaceID,
		"path":          fixture.Path,
		"encodedBytes":  fixture.EncodedBytes,
		"sha256":        fixture.SHA256,
		"fixtureBase64": fixtureBase64,
		"deadlineMs":    120000,
	})
	if err != nil {
		return projectedImageFixtureProof{}, errors.Wrapf(err, "%s projected-image fixture", action)
	}

	// Decode the Playwright value into the typed fixture proof.
	result, ok := raw.(map[string]any)
	if !ok {
		return projectedImageFixtureProof{}, errors.Errorf("unexpected fixture proof %T", raw)
	}
	proof := projectedImageFixtureProof{}
	if value, ok := result["action"].(string); ok {
		proof.action = value
	}
	if value, ok := result["path"].(string); ok {
		proof.path = value
	}
	if value, ok := result["sha256"].(string); ok {
		proof.sha256 = value
	}
	switch value := result["encodedBytes"].(type) {
	case int:
		proof.encodedBytes = int64(value)
	case int64:
		proof.encodedBytes = value
	case float64:
		proof.encodedBytes = int64(value)
	}
	return proof, nil
}

func (p *ProjectedImage) requireDedicatedRuntime() error {
	// Read the runtime topology selected during page startup.
	raw, err := p.session.Page().Evaluate(`() => {
		const marks = globalThis.__swStartupMarks ?? []
		for (let idx = marks.length - 1; idx >= 0; idx--) {
			const mark = marks[idx]
			if (mark.label === 'runtime.mode-selected') {
				return mark.detail?.mode ?? null
			}
		}
		return null
	}`)
	if err != nil {
		return errors.Wrap(err, "read GoScript runtime mode")
	}

	// Require the runtime generation named by the benchmark state boundary.
	mode, _ := raw.(string)
	if mode != "dedicated-worker" {
		return errors.Errorf("GoScript runtime mode = %q, want dedicated-worker", mode)
	}
	return nil
}

func projectedImageURL(sessionIndex uint32, spaceID string) string {
	return projectedImageFileURL(sessionIndex, spaceID, ProjectedImageFixturePath)
}

func projectedImageFileURL(sessionIndex uint32, spaceID, filePath string) string {
	return "/p/spacewave-core/fs/u/" + strconv.FormatUint(uint64(sessionIndex), 10) +
		"/so/" + url.PathEscape(spaceID) + "/-/files/-/" +
		url.PathEscape(filePath) + "?inline=1"
}

func projectedImageRoute(pageURL string) (uint32, string, error) {
	// Parse and validate the direct Drive route shape.
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return 0, "", errors.Wrap(err, "parse projected-image Drive route")
	}
	parts := strings.Split(strings.Trim(parsed.Fragment, "/"), "/")
	if len(parts) < 4 || parts[0] != "u" || parts[2] != "so" {
		return 0, "", errors.Errorf("unexpected projected-image Drive route %q", parsed.Fragment)
	}

	// Decode the session and Space identifiers used by projected requests.
	sessionIndex, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, "", errors.Wrap(err, "parse projected-image session index")
	}
	spaceID, err := url.PathUnescape(parts[3])
	if err != nil {
		return 0, "", errors.Wrap(err, "parse projected-image Space ID")
	}
	if spaceID == "" {
		return 0, "", errors.New("projected-image Space ID is empty")
	}
	return uint32(sessionIndex), spaceID, nil
}

type projectedImageFixtureProof struct {
	action       string
	path         string
	sha256       string
	encodedBytes int64
}

func (p projectedImageFixtureProof) validate(action string, fixture Fixture) error {
	if p.action != action || p.path != fixture.Path {
		return errors.Errorf("fixture proof identity differs: action=%q path=%q", p.action, p.path)
	}
	if p.sha256 != fixture.SHA256 || p.encodedBytes != fixture.EncodedBytes {
		return errors.Errorf(
			"fixture proof differs from generated bytes: bytes=%d sha256=%s",
			p.encodedBytes,
			p.sha256,
		)
	}
	return nil
}
