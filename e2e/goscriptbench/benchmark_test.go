//go:build !skip_e2e && !js

package goscriptbench

import (
	"os"
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/s4wave/spacewave/e2e/wasm"
)

const (
	benchmarkRunIDEnv             = "E2E_GOSCRIPT_STORAGE_BENCH_RUN_ID"
	benchmarkOutputRootEnv        = "E2E_GOSCRIPT_STORAGE_BENCH_OUTPUT_ROOT"
	benchmarkSpacewaveRevisionEnv = "E2E_GOSCRIPT_STORAGE_BENCH_SPACEWAVE_REVISION"
	benchmarkGoScriptRevisionEnv  = "E2E_GOSCRIPT_STORAGE_BENCH_GOSCRIPT_REVISION"
	benchmarkCPUProfileEnv        = "E2E_GOSCRIPT_STORAGE_BENCH_CPU_PROFILE"
	benchmarkBrowserEnv           = "E2E_WASM_BROWSER"
)

func TestGoScriptStorageBenchmark(t *testing.T) {
	if !benchmarkEnvEnabled(projectedImageSmokeEnv) {
		t.Skipf("set %s=true to run the GoScript storage benchmark", projectedImageSmokeEnv)
	}

	// Resolve the process identity before booting the selected browser.
	runID := requireBenchmarkEnv(t, benchmarkRunIDEnv)
	outputRoot := requireBenchmarkEnv(t, benchmarkOutputRootEnv)
	engine := requireBenchmarkEnv(t, benchmarkBrowserEnv)
	spacewaveRevision := requireBenchmarkEnv(t, benchmarkSpacewaveRevisionEnv)
	goScriptRevision := requireBenchmarkEnv(t, benchmarkGoScriptRevisionEnv)
	cpuProfile := benchmarkEnvEnabled(benchmarkCPUProfileEnv)
	if cpuProfile && engine != "chromium" {
		t.Fatal("browser CPU profiling requires Chromium")
	}

	// Run the fixed population through one isolated browser process.
	harness := newProjectedImageHarness(t, engine, true)
	supported, reason := probeBenchmarkOPFS(t, harness)
	if !supported {
		artifactDir, err := PublishEngineCapability(outputRoot, EngineCapability{
			SchemaVersion: engineCapabilitySchemaVersion,
			RunID:         runID,
			Engine:        engine,
			EngineVersion: harness.Browser().Version(),
			Capability:    engineCapabilityOPFS,
			Status:        engineCapabilityUnsupported,
			Reason:        reason,
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		if _, err := ReadEngineCapability(artifactDir); err != nil {
			t.Fatal(err.Error())
		}
		t.Logf("GoScript storage benchmark capability artifact: %s", artifactDir)
		return
	}
	workload, err := NewProjectedImage(t, harness, ProjectedImageConfig{
		RunID:             runID,
		Engine:            engine,
		SpacewaveRevision: spacewaveRevision,
		GoScriptRevision:  goScriptRevision,
		UnavailableFields: []string{},
		BrowserCPUProfile: cpuProfile,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	runner, err := NewRunner(outputRoot)
	if err != nil {
		t.Fatal(err.Error())
	}
	artifactDir, err := runner.Run(t.Context(), workload)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the published directory through the same integrity boundary as consumers.
	bundle, err := ReadArtifact(artifactDir)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bundle.Result.Metadata.RunID != runID || bundle.Result.Metadata.Engine != engine {
		t.Fatalf("published identity = %q/%q", bundle.Result.Metadata.RunID, bundle.Result.Metadata.Engine)
	}
	if bundle.Result.Metadata.SpacewaveRevision != spacewaveRevision ||
		bundle.Result.Metadata.GoScriptRevision != goScriptRevision {
		t.Fatal("published source revisions differ from the process configuration")
	}
	if cpuProfile && len(bundle.BrowserCPUProfile) == 0 {
		t.Fatal("requested Chromium CPU profile is empty")
	}
	t.Logf("GoScript storage benchmark artifact: %s", artifactDir)
}

func probeBenchmarkOPFS(t *testing.T, harness *wasm.Harness) (bool, string) {
	t.Helper()
	session := harness.NewCleanBlankSession(t)
	defer session.Release()
	page := session.Page()
	routeResult := make(chan error, 1)
	if err := page.Route(harness.BaseURL(), func(route playwright.Route) {
		routeResult <- route.Fulfill(playwright.RouteFulfillOptions{
			Body:        "<!doctype html><title>OPFS capability</title>",
			ContentType: new("text/html"),
		})
	}, 1); err != nil {
		t.Fatalf("isolate OPFS capability document: %v", err)
	}
	if _, err := page.Goto(harness.BaseURL()); err != nil {
		t.Fatalf("load OPFS capability origin: %v", err)
	}
	if err := <-routeResult; err != nil {
		t.Fatalf("serve OPFS capability document: %v", err)
	}
	raw, err := page.Evaluate(`async () => {
		if (typeof navigator.storage?.getDirectory !== 'function') {
			return {
				supported: false,
				reason: 'navigator.storage.getDirectory is unavailable',
			}
		}
		try {
			await navigator.storage.getDirectory()
			return { supported: true, reason: '' }
		} catch (error) {
			const name = error?.name ?? 'Error'
			const message = error?.message ?? String(error)
			return { supported: false, reason: name + ': ' + message }
		}
	}`)
	if err != nil {
		t.Fatalf("probe browser OPFS capability: %v", err)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("OPFS capability result has type %T", raw)
	}
	supported, ok := result["supported"].(bool)
	if !ok {
		t.Fatal("OPFS capability result is missing supported")
	}
	reason, ok := result["reason"].(string)
	if !ok {
		t.Fatal("OPFS capability result is missing reason")
	}
	if supported == (reason != "") {
		t.Fatal("OPFS capability result has inconsistent support and reason")
	}
	return supported, reason
}

func requireBenchmarkEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func benchmarkEnvEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
