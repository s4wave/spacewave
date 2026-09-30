package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/enabled"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_build "github.com/s4wave/spacewave/bldr/manifest/build"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/dex"
	"github.com/sirupsen/logrus"
)

const testStartupCacheBuilderConfigID = "test/startup-cache-builder"

var testStartupCacheBuilderState struct {
	cacheSafe        atomic.Bool
	buildCalls       atomic.Int32
	buildSubManifest atomic.Bool
}

type testStartupCacheBuilderConfig struct{}

func (c *testStartupCacheBuilderConfig) GetConfigID() string {
	return testStartupCacheBuilderConfigID
}

func (c *testStartupCacheBuilderConfig) EqualsConfig(c2 config.Config) bool {
	_, ok := c2.(*testStartupCacheBuilderConfig)
	return ok
}

func (c *testStartupCacheBuilderConfig) Validate() error {
	return nil
}

func (c *testStartupCacheBuilderConfig) SizeVT() int {
	return 0
}

func (c *testStartupCacheBuilderConfig) MarshalToSizedBufferVT(dAtA []byte) (int, error) {
	return 0, nil
}

func (c *testStartupCacheBuilderConfig) MarshalVT() ([]byte, error) {
	return nil, nil
}

func (c *testStartupCacheBuilderConfig) UnmarshalVT(data []byte) error {
	return nil
}

func (c *testStartupCacheBuilderConfig) Reset() {}

func (c *testStartupCacheBuilderConfig) MarshalJSON() ([]byte, error) {
	return []byte("{}"), nil
}

func (c *testStartupCacheBuilderConfig) UnmarshalJSON(data []byte) error {
	return nil
}

type testStartupCacheBuilder struct {
	*bus.BusController[*testStartupCacheBuilderConfig]
}

func newTestStartupCacheBuilderFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		testStartupCacheBuilderConfigID,
		testStartupCacheBuilderConfigID,
		controller.MustParseVersion("0.0.1"),
		"test startup cache builder",
		func() *testStartupCacheBuilderConfig { return &testStartupCacheBuilderConfig{} },
		func(base *bus.BusController[*testStartupCacheBuilderConfig]) (*testStartupCacheBuilder, error) {
			return &testStartupCacheBuilder{BusController: base}, nil
		},
	)
}

func (c *testStartupCacheBuilder) Execute(ctx context.Context) error {
	return nil
}

func (c *testStartupCacheBuilder) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	// Count the build call and derive the input path and bucket from the meta.
	testStartupCacheBuilderState.buildCalls.Add(1)
	builderConfig := args.GetBuilderConfig()
	meta := builderConfig.GetManifestMeta().CloneVT()
	inputPath := "main.go"
	bucketID := "built-bucket"
	if strings.HasSuffix(meta.GetManifestId(), "-child") {
		inputPath = "child.ts"
		bucketID = "built-child-bucket"
	}

	// Optionally build the child sub-manifest for the demo manifest.
	if testStartupCacheBuilderState.buildSubManifest.Load() && meta.GetManifestId() == "demo" {
		childBuilderConfig, err := configset_proto.NewControllerConfig(
			configset.NewControllerConfig(1, &testStartupCacheBuilderConfig{}),
			true,
		)
		if err != nil {
			return nil, err
		}
		childPromise, err := host.BuildSubManifest(ctx, "child", &bldr_project.ManifestConfig{
			Builder: childBuilderConfig,
		})
		if err != nil {
			return nil, err
		}
		if _, err := childPromise.Await(ctx); err != nil {
			return nil, err
		}
	}

	// Return the built manifest result.
	return bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: bucketID},
		bldr_manifest_builder.NewInputManifest([]string{inputPath}, nil),
	), nil
}

func (c *testStartupCacheBuilder) SupportsStartupManifestCache() bool {
	return testStartupCacheBuilderState.cacheSafe.Load()
}

func (c *testStartupCacheBuilder) GetSupportedPlatforms() []string {
	return nil
}

type startupCacheBlockingLookupController struct{}

func (startupCacheBlockingLookupController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

func (startupCacheBlockingLookupController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/startup-cache-blocking-lookup",
		controller.MustParseVersion("0.0.1"),
		"",
	)
}

func (startupCacheBlockingLookupController) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(dex.LookupBlockFromNetwork); !ok {
		return nil, nil
	}
	return directive.R(startupCacheBlockingLookupResolver{}, nil)
}

func (startupCacheBlockingLookupController) Close() error {
	return nil
}

type startupCacheBlockingLookupResolver struct{}

func (startupCacheBlockingLookupResolver) Resolve(
	ctx context.Context,
	_ directive.ResolverHandler,
) error {
	<-ctx.Done()
	return context.Canceled
}

// writeSettledFile writes a fixture input dated a second back, so builds the
// test starts do not see it as modified during the build.
func writeSettledFile(t *testing.T, filePath, content string) {
	// Mark the failure path on the test.
	t.Helper()

	// Write the file and backdate its modification time.
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	settled := time.Now().Add(-time.Second)
	if err := os.Chtimes(filePath, settled, settled); err != nil {
		t.Fatal(err)
	}
}

func TestValidateStartupFilesHashFallback(t *testing.T) {
	// Write the fixture input file.
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.ts")
	if err := os.WriteFile(filePath, []byte("console.log('ok');\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture identities and validate the unchanged file.
	inputManifest := bldr_manifest_builder.NewInputManifest([]string{"main.ts"}, nil)
	if err := captureFileIdentities(tmpDir, inputManifest); err != nil {
		t.Fatal(err)
	}
	if err := validateStartupFiles(tmpDir, inputManifest); err != nil {
		t.Fatalf("validate unchanged: %v", err)
	}

	// Touch the file without changing its contents and validate again.
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	nextTime := fileInfo.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(filePath, nextTime, nextTime); err != nil {
		t.Fatal(err)
	}
	if err := validateStartupFiles(tmpDir, inputManifest); err != nil {
		t.Fatalf("validate modtime-only change: %v", err)
	}

	// Change the contents and expect validation to fail.
	if err := os.WriteFile(filePath, []byte("console.log('changed');\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateStartupFiles(tmpDir, inputManifest); err == nil {
		t.Fatal("expected validation error after content change")
	}
}

func TestValidateStartupFilesInvalidatesOnlyOwningArtifacts(t *testing.T) {
	artifactInputs := map[string][]string{
		"go-core":    {"main.go", "model.proto"},
		"js-app":     {"app.ts", "model.proto"},
		"web-static": {"index.html"},
	}
	tests := []struct {
		name       string
		path       string
		content    string
		wantMisses map[string]bool
	}{
		{
			name:       "Go",
			path:       "main.go",
			content:    "package main\n// changed\n",
			wantMisses: map[string]bool{"go-core": true},
		},
		{
			name:       "TypeScript",
			path:       "app.ts",
			content:    "export const app = false;\n",
			wantMisses: map[string]bool{"js-app": true},
		},
		{
			name:       "proto",
			path:       "model.proto",
			content:    "syntax = \"proto3\";\nmessage Changed {}\n",
			wantMisses: map[string]bool{"go-core": true, "js-app": true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Write the fixture files into a fresh directory.
			tmpDir := t.TempDir()
			files := map[string]string{
				"main.go":     "package main\n",
				"app.ts":      "export const app = true;\n",
				"model.proto": "syntax = \"proto3\";\nmessage Model {}\n",
				"index.html":  "<main></main>\n",
			}
			for filePath, content := range files {
				if err := os.WriteFile(filepath.Join(tmpDir, filePath), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			// Capture identities for each artifact's input manifest.
			manifests := make(map[string]*bldr_manifest_builder.InputManifest, len(artifactInputs))
			for artifact, paths := range artifactInputs {
				inputManifest := bldr_manifest_builder.NewInputManifest(paths, nil)
				if err := captureFileIdentities(tmpDir, inputManifest); err != nil {
					t.Fatal(err)
				}
				manifests[artifact] = inputManifest
			}

			// Change one input and assert only its owning artifacts miss.
			if err := os.WriteFile(filepath.Join(tmpDir, test.path), []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			for artifact, inputManifest := range manifests {
				missed := validateStartupFiles(tmpDir, inputManifest) != nil
				if missed != test.wantMisses[artifact] {
					t.Fatalf("%s cache miss = %t, want %t", artifact, missed, test.wantMisses[artifact])
				}
			}
		})
	}
}

func TestValidateStartupFilesEscapedRelativePath(t *testing.T) {
	// Write a nested fixture file inside the temporary directory.
	tmpDir := t.TempDir()
	filePath := filepath.Join(
		tmpDir,
		"node_modules",
		"@aptre",
		"it-ws",
		"dist",
		"src",
		"duplex.js",
	)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("export const duplex = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture identities for a manifest whose path escapes the source root.
	inputManifest := bldr_manifest_builder.NewInputManifest(
		[]string{"../../../../../../../../node_modules/@aptre/it-ws/dist/src/duplex.js"},
		nil,
	)
	if err := captureFileIdentities(tmpDir, inputManifest); err != nil {
		t.Fatal(err)
	}
	if err := validateStartupFiles(tmpDir, inputManifest); err != nil {
		t.Fatalf("validate escaped path: %v", err)
	}
}

func TestValidateStartupFilesEscapedBldrDistPath(t *testing.T) {
	// Write a fixture file under the generated .bldr/src tree.
	tmpDir := t.TempDir()
	filePath := filepath.Join(
		tmpDir,
		".bldr",
		"src",
		"web",
		"bldr-react",
		"DebugInfo.tsx",
	)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("export function DebugInfo() { return null }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture identities for a manifest whose path escapes into .bldr/src.
	inputManifest := bldr_manifest_builder.NewInputManifest(
		[]string{"../../../../../../../src/web/bldr-react/DebugInfo.tsx"},
		nil,
	)
	if err := captureFileIdentities(tmpDir, inputManifest); err != nil {
		t.Fatal(err)
	}
	if err := validateStartupFiles(tmpDir, inputManifest); err != nil {
		t.Fatalf("validate escaped .bldr path: %v", err)
	}
}

func TestValidateStartupInputs(t *testing.T) {
	// Marshal a digest for an empty controller configuration.
	t.Setenv("BLDR_TEST_ENV", "expected")
	controllerConfig := &configset_proto.ControllerConfig{}
	controllerConfigDigest, err := marshalStartupConfigDigest(controllerConfig, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Record the config digest, cache format, and env var as startup inputs.
	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewControllerConfigDigestStartupInput(controllerConfigDigest),
	)
	inputManifest.AddStartupInput(newStartupCacheFormatInput())
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewEnvStartupInput("BLDR_TEST_ENV", "expected"),
	)

	// Validate the inputs against the unchanged configuration.
	if err := validateStartupInputs(controllerConfig, nil, inputManifest); err != nil {
		t.Fatalf("validate startup inputs: %v", err)
	}

	// A changed build policy must invalidate the cached result.
	policy := &bldr_manifest_build.BuildPolicy{JsMinification: enabled.Enabled_ENABLE}
	if err := validateStartupInputs(controllerConfig, policy, inputManifest); err == nil {
		t.Fatal("expected rebuild after build policy changed")
	}

	// Recording the new policy digest restores validation success.
	policyDigest, err := marshalStartupConfigDigest(controllerConfig, policy)
	if err != nil {
		t.Fatal(err)
	}
	inputManifest.StartupInputs[0] = bldr_manifest_builder.NewControllerConfigDigestStartupInput(policyDigest)
	if err := validateStartupInputs(controllerConfig, policy, inputManifest); err != nil {
		t.Fatalf("unchanged build policy: %v", err)
	}

	// A changed environment variable must invalidate the cached result.
	t.Setenv("BLDR_TEST_ENV", "changed")
	if err := validateStartupInputs(controllerConfig, policy, inputManifest); err == nil {
		t.Fatal("expected env validation error")
	}
}

func TestValidateStartupInputsRequiresCacheFormat(t *testing.T) {
	// Marshal a digest for an empty controller configuration.
	controllerConfig := &configset_proto.ControllerConfig{}
	controllerConfigDigest, err := marshalStartupConfigDigest(controllerConfig, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Record only the config digest and expect validation to fail.
	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewControllerConfigDigestStartupInput(controllerConfigDigest),
	)
	if err := validateStartupInputs(controllerConfig, nil, inputManifest); err == nil {
		t.Fatal("expected missing startup cache format marker error")
	}
}

func TestValidateStartupInputsRejectsOldCacheFormat(t *testing.T) {
	// Marshal a digest for an empty controller configuration.
	controllerConfig := &configset_proto.ControllerConfig{}
	controllerConfigDigest, err := marshalStartupConfigDigest(controllerConfig, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Record the config digest and a stale cache format marker.
	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewControllerConfigDigestStartupInput(controllerConfigDigest),
	)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewEnvStartupInput("BLDR_STARTUP_CACHE_FORMAT_V10", ""),
	)

	// Validation must fail with the missing-format-marker error.
	err = validateStartupInputs(controllerConfig, nil, inputManifest)
	if err == nil || !strings.Contains(err.Error(), "missing startup cache format marker") {
		t.Fatalf("validate startup inputs error = %v, want missing current cache format", err)
	}
}

func TestValidateStartupHookDeclaredProvenanceInvalidation(t *testing.T) {
	// Write a settled hook input file and declare its env var.
	t.Setenv("BLDR_TEST_HOOK_ENV", "declared-value")
	tmpDir := t.TempDir()
	hookInputPath := filepath.Join(tmpDir, "hook", "input.json")
	if err := os.MkdirAll(filepath.Dir(hookInputPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSettledFile(t, hookInputPath, "{\"v\":1}\n")

	// Build a builder config pointing at the fixture directory.
	controllerConfig := &configset_proto.ControllerConfig{}
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	builderConfig := &bldr_manifest_builder.BuilderConfig{ManifestMeta: meta, SourcePath: tmpDir}

	// Mirror the JS compiler folding a declared-provenance hook's inputs: the
	// declared file becomes an input file and the declared env var a startup
	// input, then generic enrichment adds the config digest and format marker.
	buildInputManifest := func() *bldr_manifest_builder.InputManifest {
		// Record the declared file and env var, then enrich the result.
		inputManifest := bldr_manifest_builder.NewInputManifest([]string{"hook/input.json"}, nil)
		inputManifest.AddStartupInput(
			bldr_manifest_builder.NewEnvStartupInput("BLDR_TEST_HOOK_ENV", os.Getenv("BLDR_TEST_HOOK_ENV")),
		)
		builderResult := bldr_manifest_builder.NewBuilderResult(
			bldr_manifest.NewManifest(meta, "dist/demo"),
			&bucket.ObjectRef{BucketId: "manifest-bucket"},
			inputManifest,
		)
		if _, err := enrichBuilderResultForStartupReuse(builderConfig, controllerConfig, builderResult, time.Now()); err != nil {
			t.Fatal(err)
		}
		return builderResult.GetInputManifest()
	}

	// Unchanged declared provenance reuses the cache.
	inputManifest := buildInputManifest()
	if err := validateStartupFiles(tmpDir, inputManifest); err != nil {
		t.Fatalf("unchanged declared file validation: %v", err)
	}
	if err := validateStartupInputs(controllerConfig, nil, inputManifest); err != nil {
		t.Fatalf("unchanged declared env validation: %v", err)
	}

	// A changed declared input file forces a rebuild.
	if err := os.WriteFile(hookInputPath, []byte("{\"v\":2}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateStartupFiles(tmpDir, inputManifest); err == nil {
		t.Fatal("expected rebuild after declared input file changed")
	}

	// A changed declared environment variable forces a rebuild.
	fresh := buildInputManifest()
	t.Setenv("BLDR_TEST_HOOK_ENV", "changed-value")
	if err := validateStartupInputs(controllerConfig, nil, fresh); err == nil {
		t.Fatal("expected rebuild after declared env var changed")
	}

	// An unset declared environment variable still participates in identity.
	if err := os.Unsetenv("BLDR_TEST_HOOK_ENV"); err != nil {
		t.Fatal(err)
	}
	unset := buildInputManifest()
	t.Setenv("BLDR_TEST_HOOK_ENV", "set-after-unset")
	if err := validateStartupInputs(controllerConfig, nil, unset); err == nil {
		t.Fatal("expected rebuild after declared env var changed from unset")
	}
}

func TestEnrichBuilderResultForStartupReuse(t *testing.T) {
	// Write a settled source file for the input manifest.
	tmpDir := t.TempDir()
	writeSettledFile(t, filepath.Join(tmpDir, "main.go"), "package main\n")

	// Build a result whose input manifest has no captured identities.
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	builderResult := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	builderConfig := &bldr_manifest_builder.BuilderConfig{
		ManifestMeta: meta,
		SourcePath:   tmpDir,
	}

	// Enrich the result for startup reuse.
	if _, err := enrichBuilderResultForStartupReuse(builderConfig, &configset_proto.ControllerConfig{}, builderResult, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Assert the enriched manifest carries the file identity and inputs.
	inputManifest := builderResult.GetInputManifest()
	if len(inputManifest.GetFiles()) != 1 {
		t.Fatalf("expected 1 file, got %d", len(inputManifest.GetFiles()))
	}
	if inputManifest.GetFiles()[0].GetIdentity() == nil {
		t.Fatal("expected captured file identity")
	}
	if len(inputManifest.GetStartupInputs()) != 2 {
		t.Fatalf("expected 2 startup inputs, got %d", len(inputManifest.GetStartupInputs()))
	}

	// Assert the digest and cache format marker startup inputs are present.
	var foundControllerDigest bool
	var foundCacheFormat bool
	for _, input := range inputManifest.GetStartupInputs() {
		if input.GetKind() == bldr_manifest_builder.InputManifest_StartupInputKind_CONTROLLER_CONFIG_DIGEST {
			foundControllerDigest = true
		}
		if input.GetKind() == bldr_manifest_builder.InputManifest_StartupInputKind_ENV_VAR &&
			input.GetKey() == startupCacheFormatEnvKey {
			foundCacheFormat = true
		}
	}
	if !foundControllerDigest {
		t.Fatal("expected controller config digest startup input")
	}
	if !foundCacheFormat {
		t.Fatal("expected startup cache format marker input")
	}
}

// TestEnrichBuilderResultRejectsInputChangedDuringBuild covers a source edit
// that lands while the compiler runs: the captured hash matches the new
// content, but the output was built from the old content.
func TestEnrichBuilderResultRejectsInputChangedDuringBuild(t *testing.T) {
	// Record the build start time and write the source file.
	tmpDir := t.TempDir()
	buildStart := time.Now()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a result whose input manifest has no captured identities.
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	builderResult := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "manifest-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	builderConfig := &bldr_manifest_builder.BuilderConfig{
		ManifestMeta: meta,
		SourcePath:   tmpDir,
	}
	controllerConfig := &configset_proto.ControllerConfig{}

	// Enrich against the build start time and expect the changed input.
	changedInput, err := enrichBuilderResultForStartupReuse(builderConfig, controllerConfig, builderResult, buildStart)
	if err != nil {
		t.Fatal(err)
	}

	// Startup validation must reject the stale result.
	if changedInput != "main.go" {
		t.Fatalf("expected main.go reported as changed, got %q", changedInput)
	}
	if err := validateStartupInputs(controllerConfig, nil, builderResult.GetInputManifest()); err == nil {
		t.Fatal("expected next startup to reject the result")
	}
}

func TestControllerStartupCacheHitSkipsBuild(t *testing.T) {
	// Prepare the source file and a startup builder result.
	tmpDir := t.TempDir()
	writeSettledFile(t, filepath.Join(tmpDir, "main.go"), "package main\n")
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)

	// Run the controller with the cached result and assert no build ran.
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, true)
	if buildCalls != 0 {
		t.Fatalf("expected 0 build calls, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "startup-bucket" {
		t.Fatal("expected startup builder result to be reused")
	}
}

func TestControllerPersistsAndReusesSubManifestResults(t *testing.T) {
	// Write the parent and child source files.
	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main.go")
	childPath := filepath.Join(tmpDir, "child.ts")
	writeSettledFile(t, mainPath, "package main\n")
	writeSettledFile(t, childPath, "export const child = true;\n")

	// Enable sub-manifest builds for this test.
	testStartupCacheBuilderState.buildSubManifest.Store(true)
	t.Cleanup(func() {
		testStartupCacheBuilderState.buildSubManifest.Store(false)
	})

	// Run the initial build and assert both parent and child were built.
	startupResult, buildCalls := runStartupExecuteTest(t, tmpDir, nil, true)
	if buildCalls != 2 {
		t.Fatalf("initial build calls = %d, want parent and child", buildCalls)
	}
	if startupResult.GetSubManifestResults()["child"] == nil {
		t.Fatal("parent result did not persist child builder result")
	}

	// Assert the parent input manifest retains the child startup input.
	var childStartupFile bool
	for _, inputFile := range startupResult.GetInputManifest().GetFiles() {
		if inputFile.GetPath() == "child.ts" && inputFile.GetStartupOnly() {
			childStartupFile = true
		}
	}
	if !childStartupFile {
		t.Fatal("parent result did not retain child input for startup validation")
	}

	// An unchanged rerun must hit the cache for both manifests.
	_, buildCalls = runStartupExecuteTest(t, tmpDir, startupResult, true)
	if buildCalls != 0 {
		t.Fatalf("unchanged build calls = %d, want 0", buildCalls)
	}

	// A parent-only mutation rebuilds the parent only.
	if err := os.WriteFile(mainPath, []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, buildCalls = runStartupExecuteTest(t, tmpDir, startupResult, true)
	if buildCalls != 1 {
		t.Fatalf("parent-only mutation build calls = %d, want parent only", buildCalls)
	}

	// A child mutation rebuilds both parent and child.
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("export const child = false;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, buildCalls = runStartupExecuteTest(t, tmpDir, startupResult, true)
	if buildCalls != 2 {
		t.Fatalf("child mutation build calls = %d, want parent and child", buildCalls)
	}
}

func TestControllerStartupCacheHitPublishesLifecycleStatusOrdering(t *testing.T) {
	// Prepare the source file, cached result, and lifecycle sink.
	tmpDir := t.TempDir()
	writeSettledFile(t, filepath.Join(tmpDir, "main.go"), "package main\n")
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	sink := newRecordingLifecycleSink()

	// Run the controller and record the lifecycle statuses.
	result, buildCalls := runStartupExecuteWithLifecycle(
		t,
		tmpDir,
		startupBuilderResult,
		true,
		false,
		sink,
	)
	if buildCalls != 0 {
		t.Fatalf("expected 0 build calls, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "startup-bucket" {
		t.Fatal("expected startup builder result to be reused")
	}

	// Assert the lifecycle summaries and the final cache-hit status.
	assertLifecycleSummaries(t, sink.nonEmptySnapshot(), []string{
		"queued",
		"starting builder controller",
		"startup cache hit",
		"build complete",
	})
	done := sink.nonEmptySnapshot()[3]
	if done.State != ManifestBuilderLifecycleStateDone || !done.CacheHit || done.FullRebuild || done.HotRebuild {
		t.Fatalf("unexpected final startup-cache status: %#v", done)
	}
}

func TestControllerStartupFileMissRebuilds(t *testing.T) {
	// Capture a startup result, then change the source file.
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	if err := os.WriteFile(filePath, []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run the controller and assert the rebuild happened.
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, true)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerStartupFileMissPublishesFullBuildLifecycle(t *testing.T) {
	// Capture a startup result, then change the source file.
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	if err := os.WriteFile(filePath, []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run the controller with a lifecycle sink and record the statuses.
	sink := newRecordingLifecycleSink()
	result, buildCalls := runStartupExecuteWithLifecycle(
		t,
		tmpDir,
		startupBuilderResult,
		true,
		false,
		sink,
	)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}

	// Assert the full-rebuild lifecycle summaries and final statuses.
	assertLifecycleSummaries(t, sink.nonEmptySnapshot(), []string{
		"queued",
		"starting builder controller",
		"full rebuild",
		"build complete",
	})
	running := sink.nonEmptySnapshot()[2]
	if running.State != ManifestBuilderLifecycleStateRunning || !running.FullRebuild || running.HotRebuild || running.CacheHit {
		t.Fatalf("unexpected full rebuild running status: %#v", running)
	}
	done := sink.nonEmptySnapshot()[3]
	if done.State != ManifestBuilderLifecycleStateDone || !done.FullRebuild || done.HotRebuild || done.CacheHit {
		t.Fatalf("unexpected full rebuild done status: %#v", done)
	}
}

func TestControllerFileChangeRebuildReplacesResultPromiseAndPublishesReason(t *testing.T) {
	// Write the watched source file.
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Bound the test with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the testbed and register the test builder factory.
	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Reset the builder state and register the test builder factory.
	testStartupCacheBuilderState.cacheSafe.Store(true)
	testStartupCacheBuilderState.buildCalls.Store(0)
	tb.GetStaticResolver().AddFactory(newTestStartupCacheBuilderFactory(tb.GetBus()))

	// Construct the watching controller and attach the lifecycle sink.
	builderControllerConfig := newTestBuilderControllerProto(t)
	builderConfig := &bldr_manifest_builder.BuilderConfig{
		ManifestMeta: bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1),
		SourcePath:   tmpDir,
	}
	controllerConfig := NewConfig(
		builderConfig,
		builderControllerConfig,
		nil,
		true,
		nil,
	)
	ctrl := NewController(tb.GetLogger(), tb.GetBus(), controllerConfig)
	sink := newRecordingLifecycleSink()
	ctrl.SetManifestBuilderLifecycleSink(sink)

	// Run the controller and await the initial build result.
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

	// Await the initial build result from the controller.
	firstResult, err := ctrl.GetResultPromise().Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if firstResult.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected initial build result")
	}

	// Hold the first promise and wait for the watch to settle.
	firstPromise, waitCh := ctrl.GetResultPromise().GetPromise()
	if firstPromise == nil {
		t.Fatal("expected first result promise")
	}
	sink.waitFor(t, ctx, func(status ManifestBuilderLifecycleStatus) bool {
		return status.Summary == "watching for changes"
	})

	// Change the watched file until the result promise is replaced.
	writeWatchedFileUntilPromiseReplaced(t, ctx, filePath, waitCh)
	secondPromise, _ := ctrl.GetResultPromise().GetPromise()
	if secondPromise == nil {
		t.Fatal("expected second result promise")
	}
	if secondPromise == firstPromise {
		t.Fatal("expected result promise to be replaced on rebuild")
	}

	// Await the rebuilt result and assert the revision advanced.
	secondResult, err := secondPromise.Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if secondResult.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
	if firstResult.GetManifest().GetMeta().GetRev() != 1 || secondResult.GetManifest().GetMeta().GetRev() != 2 {
		t.Fatal("a watch rebuild must publish a newer selectable revision")
	}
	if builderConfig.GetManifestMeta().GetRev() != 1 {
		t.Fatal("a watch rebuild mutated the shared builder configuration")
	}

	// Assert the hot-rebuild lifecycle status and build call count.
	hot := sink.waitFor(t, ctx, func(status ManifestBuilderLifecycleStatus) bool {
		return status.HotRebuild && status.DependencyRebuildReason == changedFilesSummary(1)
	})
	if hot.State != ManifestBuilderLifecycleStateRunning || hot.FullRebuild || hot.CacheHit {
		t.Fatalf("unexpected hot rebuild lifecycle status: %#v", hot)
	}
	if got := testStartupCacheBuilderState.buildCalls.Load(); got != 2 {
		t.Fatalf("build calls = %d, want 2", got)
	}

	// Cancel the context and wait for Execute to exit.
	cancel()
	if execErr := waitForControllerExecuteExit(t, errCh); execErr != nil && execErr != context.Canceled {
		t.Fatalf("execute: %v", execErr)
	}
}

func TestControllerStartupEnvMissRebuilds(t *testing.T) {
	// Capture a startup result that records the old env value.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BLDR_TEST_ENV", "old")
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	startupBuilderResult.GetInputManifest().AddStartupInput(
		bldr_manifest_builder.NewEnvStartupInput("BLDR_TEST_ENV", "old"),
	)

	// Change the env value and assert the rebuild happened.
	t.Setenv("BLDR_TEST_ENV", "new")
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, true)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerStartupUnsafeBuilderRebuilds(t *testing.T) {
	// Capture a startup result for a builder that disallows cache reuse.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)

	// Run the controller with cache reuse disabled and assert the rebuild.
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, false)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerStartupMissingManifestRebuilds(t *testing.T) {
	// Write the source file for the build.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build the testbed for the stored manifest lookup.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the testbed with a debug logger.
	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Corrupt the cached manifest root ref so the lookup misses.
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStoredStartupBuilderResult(t, tb, tmpDir, builderControllerConfig)
	startupBuilderResult.ManifestRef.ManifestRef = startupBuilderResult.GetManifestRef().GetManifestRef().CloneVT()
	startupBuilderResult.ManifestRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff

	// Run the controller and assert the rebuild happened.
	result, buildCalls := runStartupExecuteWithTestbed(
		t,
		tb,
		tmpDir,
		startupBuilderResult,
		true,
		tb.GetWorldEngineID(),
	)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerStartupManifestBucketMismatchRebuilds(t *testing.T) {
	// Write the source file for the build.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build the testbed for the stored manifest lookup.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the testbed with a debug logger.
	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Change the cached manifest bucket ID so the lookup misses.
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStoredStartupBuilderResult(t, tb, tmpDir, builderControllerConfig)
	startupBuilderResult.ManifestRef.ManifestRef = startupBuilderResult.GetManifestRef().GetManifestRef().CloneVT()
	startupBuilderResult.ManifestRef.ManifestRef.BucketId = "other-bucket"

	// Run the controller and assert the rebuild happened.
	result, buildCalls := runStartupExecuteWithTestbed(
		t,
		tb,
		tmpDir,
		startupBuilderResult,
		true,
		tb.GetWorldEngineID(),
	)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestValidateStartupManifestAvailabilitySkipsUnavailableLookupBucketBlock(t *testing.T) {
	// Write the source file for the build.
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build the testbed for the stored manifest lookup.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the testbed with a debug logger.
	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Install a controller that blocks every network bucket lookup.
	ctrlRel, err := tb.GetBus().AddController(ctx, startupCacheBlockingLookupController{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRel()

	// Store the cached manifest in the testbed World.
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStoredStartupBuilderResult(t, tb, tmpDir, builderControllerConfig)
	cachedBucketID := startupBuilderResult.GetManifestRef().GetManifestRef().GetBucketId()

	// Apply a lookup-backed bucket config for the cached bucket.
	bucketLkConfig, err := bucket.NewLookupConfig(configset.NewControllerConfig(1, &lookup_concurrent.Config{
		NotFoundBehavior: lookup_concurrent.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE_WAIT,
	}))
	if err != nil {
		t.Fatal(err)
	}
	bucketConf, err := bucket.NewConfig(cachedBucketID, 2, bucketLkConfig)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = tb.GetVolume().ApplyBucketConfig(ctx, bucketConf)
	if err != nil {
		t.Fatal(err)
	}

	// Load the bucket lookup so the bucket config is locally available.
	waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
	lookupHandle, _, lookupHandleRef, err := bucket_lookup.ExBuildBucketLookup(waitCtx, tb.GetBus(), false, cachedBucketID, nil)
	waitCancel()
	if err != nil {
		t.Fatal(err)
	}
	defer lookupHandleRef.Release()
	if lookupHandle.GetBucketConfig() == nil {
		t.Fatal("lookup bucket config was not loaded")
	}

	// Corrupt the cached manifest root ref so the lookup misses.
	startupBuilderResult.ManifestRef.ManifestRef = startupBuilderResult.GetManifestRef().GetManifestRef().CloneVT()
	startupBuilderResult.ManifestRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff

	// Build a controller configured with the cached startup result.
	builderConfig := &bldr_manifest_builder.BuilderConfig{
		ManifestMeta: bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1),
		SourcePath:   tmpDir,
		EngineId:     tb.GetWorldEngineID(),
	}
	controllerConfig := NewConfig(
		builderConfig,
		builderControllerConfig,
		nil,
		false,
		startupBuilderResult,
	)
	ctrl := NewController(tb.GetLogger(), tb.GetBus(), controllerConfig)

	// Validate availability with a short timeout and inspect the reason.
	validateCtx, validateCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer validateCancel()
	reason, err := ctrl.validateStartupManifestAvailability(validateCtx, tb.GetLogger(), startupBuilderResult)
	if err != nil {
		t.Fatal(err)
	}
	if reason == "" {
		t.Fatal("expected startup cache miss reason")
	}
	if strings.Contains(reason, context.DeadlineExceeded.Error()) {
		t.Fatalf("startup validation waited for network lookup: %s", reason)
	}
	if !strings.Contains(reason, block.ErrNotFound.Error()) {
		t.Fatalf("startup validation reason = %q, want block not found", reason)
	}
}

func buildStartupBuilderResult(
	t *testing.T,
	sourcePath string,
	controllerConfig *configset_proto.ControllerConfig,
) *bldr_manifest_builder.BuilderResult {
	// Mark the failure path on the test.
	t.Helper()

	// Build a result and enrich it for startup reuse.
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	builderResult := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "startup-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	if _, err := enrichBuilderResultForStartupReuse(
		&bldr_manifest_builder.BuilderConfig{
			ManifestMeta: meta,
			SourcePath:   sourcePath,
		},
		controllerConfig,
		builderResult,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	return builderResult
}

func buildStoredStartupBuilderResult(
	t *testing.T,
	tb *testbed.Testbed,
	sourcePath string,
	controllerConfig *configset_proto.ControllerConfig,
) *bldr_manifest_builder.BuilderResult {
	// Mark the failure path on the test.
	t.Helper()

	// Write the dist output into an in-memory filesystem.
	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	distFS := memfs.New()
	if err := distFS.MkdirAll("dist", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := distFS.Create("dist/demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("demo")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Commit the manifest to the testbed World.
	manifest, manifestRef, err := tb.CreateManifestWithBilly(
		tb.GetContext(),
		meta,
		"dist/demo",
		distFS,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Build the result and enrich it for startup reuse.
	builderResult := bldr_manifest_builder.NewBuilderResult(
		manifest,
		manifestRef.GetManifestRef(),
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	if _, err := enrichBuilderResultForStartupReuse(
		&bldr_manifest_builder.BuilderConfig{
			ManifestMeta: meta,
			SourcePath:   sourcePath,
			EngineId:     tb.GetWorldEngineID(),
		},
		controllerConfig,
		builderResult,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
	return builderResult
}

func newTestBuilderControllerProto(t *testing.T) *configset_proto.ControllerConfig {
	// Mark the failure path on the test.
	t.Helper()

	// Wrap the test builder config in a controller config proto.
	builderControllerConfig, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(1, &testStartupCacheBuilderConfig{}),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	return builderControllerConfig
}

func runStartupExecuteTest(
	t *testing.T,
	sourcePath string,
	startupBuilderResult *bldr_manifest_builder.BuilderResult,
	cacheSafe bool,
) (*bldr_manifest_builder.BuilderResult, int32) {
	// Mark the failure path on the test.
	t.Helper()

	// Build a fresh testbed for the controller run.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the testbed with a debug logger.
	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Delegate to the testbed runner.
	return runStartupExecuteWithTestbed(
		t,
		tb,
		sourcePath,
		startupBuilderResult,
		cacheSafe,
		"",
	)
}

func runStartupExecuteWithTestbed(
	t *testing.T,
	tb *testbed.Testbed,
	sourcePath string,
	startupBuilderResult *bldr_manifest_builder.BuilderResult,
	cacheSafe bool,
	engineID string,
) (*bldr_manifest_builder.BuilderResult, int32) {
	// Mark the failure path on the test.
	t.Helper()

	// Reset the builder state and register the factories.
	testStartupCacheBuilderState.cacheSafe.Store(cacheSafe)
	testStartupCacheBuilderState.buildCalls.Store(0)
	tb.GetStaticResolver().AddFactory(newTestStartupCacheBuilderFactory(tb.GetBus()))
	tb.GetStaticResolver().AddFactory(NewFactory(tb.GetBus()))
	ctx := tb.GetContext()

	// Wrap the test builder config in a controller config proto.
	builderControllerConfig, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(1, &testStartupCacheBuilderConfig{}),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Build the controller configuration from the builder config.
	builderConfig := &bldr_manifest_builder.BuilderConfig{
		ManifestMeta:   bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1),
		SourcePath:     sourcePath,
		EngineId:       engineID,
		LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
	}
	controllerConfig := NewConfig(
		builderConfig,
		builderControllerConfig,
		nil,
		false,
		startupBuilderResult,
	)

	// Run the controller and await its result promise.
	ctrl := NewController(tb.GetLogger(), tb.GetBus(), controllerConfig)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

	// Await the build result from the controller.
	result, err := ctrl.GetResultPromise().Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if execErr := <-errCh; execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	return result, testStartupCacheBuilderState.buildCalls.Load()
}

func runStartupExecuteWithLifecycle(
	t *testing.T,
	sourcePath string,
	startupBuilderResult *bldr_manifest_builder.BuilderResult,
	cacheSafe bool,
	watch bool,
	sink *recordingLifecycleSink,
) (*bldr_manifest_builder.BuilderResult, int32) {
	// Mark the failure path on the test.
	t.Helper()

	// Build a fresh testbed for the controller run.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the testbed with a debug logger.
	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Reset the builder state and register the test builder factory.
	testStartupCacheBuilderState.cacheSafe.Store(cacheSafe)
	testStartupCacheBuilderState.buildCalls.Store(0)
	tb.GetStaticResolver().AddFactory(newTestStartupCacheBuilderFactory(tb.GetBus()))

	// Build the controller configuration from the builder config.
	builderControllerConfig := newTestBuilderControllerProto(t)
	builderConfig := &bldr_manifest_builder.BuilderConfig{
		ManifestMeta: bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1),
		SourcePath:   sourcePath,
	}
	controllerConfig := NewConfig(
		builderConfig,
		builderControllerConfig,
		nil,
		watch,
		startupBuilderResult,
	)

	// Run the controller with the lifecycle sink and await the result.
	ctrl := NewController(tb.GetLogger(), tb.GetBus(), controllerConfig)
	ctrl.SetManifestBuilderLifecycleSink(sink)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

	// Await the build result from the controller.
	result, err := ctrl.GetResultPromise().Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !watch {
		if execErr := <-errCh; execErr != nil {
			t.Fatalf("execute: %v", execErr)
		}
	}
	return result, testStartupCacheBuilderState.buildCalls.Load()
}

func assertLifecycleSummaries(t *testing.T, statuses []ManifestBuilderLifecycleStatus, want []string) {
	// Mark the failure path on the test.
	t.Helper()

	// Compare the status count and each summary in order.
	if len(statuses) != len(want) {
		t.Fatalf("lifecycle status count = %d, want %d: %#v", len(statuses), len(want), statuses)
	}
	for i, status := range statuses {
		if status.Summary != want[i] {
			t.Fatalf("lifecycle status %d summary = %q, want %q; statuses=%#v", i, status.Summary, want[i], statuses)
		}
	}
}

func writeWatchedFileUntilPromiseReplaced(
	t *testing.T,
	ctx context.Context,
	filePath string,
	waitCh <-chan struct{},
) {
	// Mark the failure path on the test.
	t.Helper()

	// Set up the retry ticker and the replacement timeout.
	retry := time.NewTicker(150 * time.Millisecond)
	defer retry.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	// Write successive contents until the watch replaces the promise.
	for i := 1; ; i++ {
		content := []byte("package main\n// changed " + strconv.Itoa(i) + "\n")
		if err := os.WriteFile(filePath, content, 0o644); err != nil {
			t.Fatal(err)
		}
		select {
		case <-waitCh:
			return
		case <-ctx.Done():
			t.Fatalf("context canceled before result promise replacement: %v", ctx.Err())
		case <-timeout.C:
			t.Fatal("timed out waiting for result promise replacement")
		case <-retry.C:
		}
	}
}

func waitForControllerExecuteExit(t *testing.T, errCh <-chan error) error {
	// Mark the failure path on the test.
	t.Helper()

	// Wait for the controller Execute error or time out.
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	// Return the Execute error or fail on timeout.
	select {
	case err := <-errCh:
		return err
	case <-timeout.C:
		t.Fatal("timed out waiting for controller Execute to exit")
	}
	return nil
}
