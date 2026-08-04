//go:build !js

package goscriptbench

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

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
		"decoded-image-entry",
	}
	if !slices.Equal(metadata.State.Recreated, wantRecreated) {
		t.Fatalf("recreated state = %v, want %v", metadata.State.Recreated, wantRecreated)
	}
}

func newProjectedImageSmoke(t *testing.T) *ProjectedImage {
	t.Helper()
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(projectedImageSmokeEnv)), "true") {
		t.Skipf("set %s=true to run the retained-OPFS browser smoke", projectedImageSmokeEnv)
	}
	t.Setenv(wasm.E2EWasmCompilerEnv, string(wasm.E2EWasmCompilerGoScript))
	t.Setenv(wasm.E2EWasmWorkerModeEnv, string(wasm.WorkerModeDedicated))

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	harness, err := wasm.Boot(
		t.Context(),
		logrus.NewEntry(logger),
		wasm.WithSessionHarness(),
		wasm.WithGoScriptBrowserStartup(),
		wasm.WithWorkerMode(wasm.WorkerModeDedicated),
		wasm.WithBrowserName("chromium"),
		wasm.WithStartupBuildCache(true),
		wasm.WithManifestBuildTimeout(20*time.Minute),
	)
	if err != nil {
		t.Fatalf("boot GoScript browser harness: %v", err)
	}
	t.Cleanup(harness.Release)
	if err := harness.LaunchBrowser(); err != nil {
		t.Fatalf("launch Chromium: %v", err)
	}

	workload, err := NewProjectedImage(t, harness, ProjectedImageConfig{
		RunID:             "projected-image-smoke",
		Engine:            "chromium",
		SpacewaveRevision: "test-spacewave-revision",
		GoScriptRevision:  "test-goscript-revision",
		UnavailableFields: []string{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return workload
}
