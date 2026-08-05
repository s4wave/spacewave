//go:build !js

package goscriptbench

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	trace_service "github.com/s4wave/spacewave/core/trace/service"
	wasm "github.com/s4wave/spacewave/e2e/wasm"
	"github.com/sirupsen/logrus"
)

const projectedImageSmokeEnv = "E2E_GOSCRIPT_STORAGE_BENCH"

func TestProjectedImageSetupScriptCompiles(t *testing.T) {
	scripts, err := wasm.CompileTestScripts(".", t.TempDir())
	if err != nil {
		t.Fatal(err.Error())
	}
	if scripts[projectedImageSetupScript] == "" {
		t.Fatalf("compiled scripts omit %s", projectedImageSetupScript)
	}
	if scripts[projectedImageMeasureScript] == "" {
		t.Fatalf("compiled scripts omit %s", projectedImageMeasureScript)
	}
}

func TestProjectedImageFixtureProofRequiresActionIdentity(t *testing.T) {
	fixture := Fixture{
		Path:         ProjectedImageFixturePath,
		SHA256:       "3470475e663fab4c571c4c1f3857c5bcad4902ad1058ed4cfae96b3bf5127724",
		EncodedBytes: 4_198_217,
	}
	proof := projectedImageFixtureProof{
		action:       "upload",
		path:         fixture.Path,
		sha256:       fixture.SHA256,
		encodedBytes: fixture.EncodedBytes,
	}
	if err := proof.validate("upload", fixture); err != nil {
		t.Fatal(err.Error())
	}
	if err := proof.validate("verify", fixture); err == nil {
		t.Fatal("fixture proof accepted a different action")
	}
}

func TestProjectedImageSetupVerifiesUpload(t *testing.T) {
	workload := newProjectedImageSmoke(t)
	metadata, err := workload.Setup(t.Context())
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := metadata.Validate(); err != nil {
		t.Fatal(err.Error())
	}
	if metadata.Fixture.EncodedBytes != 4_198_217 || metadata.Fixture.SHA256 != "3470475e663fab4c571c4c1f3857c5bcad4902ad1058ed4cfae96b3bf5127724" {
		t.Fatalf("stored fixture identity = %d/%s", metadata.Fixture.EncodedBytes, metadata.Fixture.SHA256)
	}
	if len(workload.priorWorkers) == 0 {
		t.Fatal("setup recorded no dedicated worker")
	}
}

func TestProjectedImageRestartRetainsFixture(t *testing.T) {
	workload := newProjectedImageSmoke(t)
	metadata, err := workload.Setup(t.Context())
	if err != nil {
		t.Fatal(err.Error())
	}
	priorContext := workload.session.BrowserContext()
	priorPage := workload.session.Page()
	priorWorkers := slices.Clone(workload.priorWorkers)

	if err := workload.Restart(t.Context(), SampleRequest{Kind: SampleKindWarmup, Number: 1}); err != nil {
		t.Fatal(err.Error())
	}
	if workload.session.BrowserContext() != priorContext {
		t.Fatal("restart replaced the retained BrowserContext")
	}
	if workload.session.Page() == priorPage {
		t.Fatal("restart retained the prior page")
	}
	if len(workload.priorWorkers) == 0 {
		t.Fatal("restart recorded no dedicated worker")
	}
	for _, worker := range workload.priorWorkers {
		if slices.Contains(priorWorkers, worker) {
			t.Fatal("restart retained a prior dedicated worker")
		}
	}

	// Require the complete cache boundary published for every sample.
	wantRetained := []string{
		"browser-process",
		"browser-context",
		"service-worker-state",
		"module-cache",
		"opfs-files",
		"operating-system-page-cache",
	}
	if !slices.Equal(metadata.State.Retained, wantRetained) {
		t.Fatalf("retained state = %v, want %v", metadata.State.Retained, wantRetained)
	}
	wantRecreated := []string{
		"page",
		"dedicated-worker",
		"go-runtime",
		"decoded-block-cache",
		"opfs-segment-cache",
		"world-state",
		"response-cache-key",
		"resource-sdk-connection",
		"decoded-image-entry",
	}
	if !slices.Equal(metadata.State.Recreated, wantRecreated) {
		t.Fatalf("recreated state = %v, want %v", metadata.State.Recreated, wantRecreated)
	}
}

func TestProjectedImageMeasureUntracedProjectedFile(t *testing.T) {
	workload := newProjectedImageSmoke(t)
	if _, err := workload.Setup(t.Context()); err != nil {
		t.Fatal(err.Error())
	}
	corruptPath := uploadCorruptProjectedImage(t, workload)
	request := SampleRequest{Kind: SampleKindWarmup, Number: 1}
	if _, err := workload.MeasureUntraced(t.Context(), request); err == nil {
		t.Fatal("sample was measured before a runtime restart")
	}
	if err := workload.Restart(t.Context(), request); err != nil {
		t.Fatal(err.Error())
	}

	sample, err := workload.MeasureUntraced(t.Context(), request)
	if err != nil {
		t.Fatal(err.Error())
	}
	if sample.ID != "warmup-1" {
		t.Fatalf("sample ID = %q, want warmup-1", sample.ID)
	}
	if err := workload.ValidateUntraced(t.Context(), request, sample); err != nil {
		t.Fatal(err.Error())
	}

	if err := workload.Restart(t.Context(), request); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := workload.MeasureUntraced(t.Context(), request); err == nil {
		t.Fatal("reused sample identity was measured")
	}
	if _, err := workload.MeasureUntraced(t.Context(), SampleRequest{
		Kind:   SampleKindWarmup,
		Number: 1,
		Trace:  true,
	}); err == nil {
		t.Fatal("traced scalar request was measured")
	}

	for _, invalid := range []struct {
		name   string
		path   string
		width  int
		height int
	}{
		{
			name:   "missing",
			path:   "goscriptbench-missing-image.png",
			width:  workload.metadata.Fixture.Width,
			height: workload.metadata.Fixture.Height,
		},
		{
			name:   "corrupt",
			path:   corruptPath,
			width:  workload.metadata.Fixture.Width,
			height: workload.metadata.Fixture.Height,
		},
		{
			name:   "dimensions",
			path:   workload.metadata.Fixture.Path,
			width:  workload.metadata.Fixture.Width + 1,
			height: workload.metadata.Fixture.Height,
		},
	} {
		projectedURL := workload.harness.BaseURL() +
			projectedImageFileURL(workload.sessionIndex, workload.spaceID, invalid.path) +
			"&sample=" + invalid.name
		if _, err := workload.measureProjectedImageURL(
			t.Context(),
			invalid.name,
			projectedURL,
			invalid.width,
			invalid.height,
		); err == nil {
			t.Fatalf("%s projected image was measured", invalid.name)
		}
	}
}

func TestProjectedImageCapturesDiagnosticEvidence(t *testing.T) {
	workload := newProjectedImageDiagnosticSmoke(t)
	metadata, err := workload.Setup(t.Context())
	if err != nil {
		t.Fatal(err.Error())
	}
	request := SampleRequest{Kind: SampleKindDiagnostic, Number: 1, Trace: true}
	if err := workload.Restart(t.Context(), request); err != nil {
		t.Fatal(err.Error())
	}
	measurement, err := workload.Measure(t.Context(), request)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := workload.Validate(t.Context(), request, measurement.Sample); err != nil {
		t.Fatal(err.Error())
	}
	if err := measurement.Validate(request, metadata); err != nil {
		t.Fatal(err.Error())
	}
	if len(measurement.RuntimeTrace) == 0 {
		t.Fatal("diagnostic runtime trace is empty")
	}
	if len(measurement.BrowserCPUProfile) == 0 {
		t.Fatal("diagnostic Chromium CPU profile is empty")
	}
}

func uploadCorruptProjectedImage(t *testing.T, workload *ProjectedImage) string {
	t.Helper()
	data := []byte("corrupt projected image")
	digest := sha256.Sum256(data)
	fixture := Fixture{
		Path:         "goscriptbench-corrupt-image.png",
		SHA256:       hex.EncodeToString(digest[:]),
		EncodedBytes: int64(len(data)),
	}
	proof, err := workload.runFixtureScript(
		"upload",
		base64.StdEncoding.EncodeToString(data),
		fixture,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := proof.validate("upload", fixture); err != nil {
		t.Fatal(err.Error())
	}
	return fixture.Path
}

func newProjectedImageSmoke(t *testing.T) *ProjectedImage {
	t.Helper()
	return newProjectedImageSmokeMode(t, false)
}

func newProjectedImageDiagnosticSmoke(t *testing.T) *ProjectedImage {
	t.Helper()
	return newProjectedImageSmokeMode(t, true)
}

func newProjectedImageSmokeMode(t *testing.T, diagnostic bool) *ProjectedImage {
	t.Helper()
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(projectedImageSmokeEnv)), "true") {
		t.Skipf("set %s=true to run the retained-OPFS browser smoke", projectedImageSmokeEnv)
	}
	harness := newProjectedImageHarness(t, "chromium", diagnostic)

	workload, err := NewProjectedImage(t, harness, ProjectedImageConfig{
		RunID:             "projected-image-smoke",
		Engine:            "chromium",
		SpacewaveRevision: "test-spacewave-revision",
		GoScriptRevision:  "test-goscript-revision",
		UnavailableFields: []string{},
		BrowserCPUProfile: diagnostic,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return workload
}

func newProjectedImageHarness(t *testing.T, engine string, trace bool) *wasm.Harness {
	t.Helper()
	t.Setenv(wasm.E2EWasmCompilerEnv, string(wasm.E2EWasmCompilerGoScript))
	t.Setenv(wasm.E2EWasmWorkerModeEnv, string(wasm.WorkerModeDedicated))

	options := []wasm.Option{
		wasm.WithSessionHarness(),
		wasm.WithGoScriptBrowserStartup(),
		wasm.WithWorkerMode(wasm.WorkerModeDedicated),
		wasm.WithBrowserName(engine),
		wasm.WithStartupBuildCache(true),
		wasm.WithManifestBuildTimeout(20 * time.Minute),
	}
	if trace {
		t.Setenv(wasm.E2EWasmGoScriptRuntimeTraceEnv, "true")
		options = append(options, wasm.WithConfigMutator(trace_service.InjectTraceConfig))
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	harness, err := wasm.Boot(
		t.Context(),
		logrus.NewEntry(logger),
		options...,
	)
	if err != nil {
		t.Fatalf("boot GoScript browser harness: %v", err)
	}
	t.Cleanup(harness.Release)
	if err := harness.LaunchBrowser(); err != nil {
		t.Fatalf("launch %s: %v", engine, err)
	}
	return harness
}
