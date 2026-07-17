package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
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
	buildHookMtx     sync.Mutex
	buildHook        func(context.Context, int32) error
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
	buildCall := testStartupCacheBuilderState.buildCalls.Add(1)
	testStartupCacheBuilderState.buildHookMtx.Lock()
	buildHook := testStartupCacheBuilderState.buildHook
	testStartupCacheBuilderState.buildHookMtx.Unlock()
	if buildHook != nil {
		if err := buildHook(ctx, buildCall); err != nil {
			return nil, err
		}
	}
	builderConfig := args.GetBuilderConfig()
	meta := builderConfig.GetManifestMeta().CloneVT()
	inputPath := "main.go"
	bucketID := "built-bucket"
	if strings.HasSuffix(meta.GetManifestId(), "-child") {
		inputPath = "child.ts"
		bucketID = "built-child-bucket"
	}
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

func TestValidateStartupFilesHashFallback(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.ts")
	if err := os.WriteFile(filePath, []byte("console.log('ok');\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inputManifest := bldr_manifest_builder.NewInputManifest([]string{"main.ts"}, nil)
	if err := captureFileIdentities(tmpDir, inputManifest); err != nil {
		t.Fatal(err)
	}
	if err := validateStartupFiles(tmpDir, inputManifest); err != nil {
		t.Fatalf("validate unchanged: %v", err)
	}

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

			manifests := make(map[string]*bldr_manifest_builder.InputManifest, len(artifactInputs))
			for artifact, paths := range artifactInputs {
				inputManifest := bldr_manifest_builder.NewInputManifest(paths, nil)
				if err := captureFileIdentities(tmpDir, inputManifest); err != nil {
					t.Fatal(err)
				}
				manifests[artifact] = inputManifest
			}

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
	t.Setenv("BLDR_TEST_ENV", "expected")
	controllerConfig := &configset_proto.ControllerConfig{}
	controllerConfigDigest, err := marshalControllerConfigDigest(controllerConfig)
	if err != nil {
		t.Fatal(err)
	}

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewControllerConfigDigestStartupInput(controllerConfigDigest),
	)
	inputManifest.AddStartupInput(newStartupCacheFormatInput())
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewEnvStartupInput("BLDR_TEST_ENV", "expected"),
	)

	if err := validateStartupInputs(controllerConfig, inputManifest); err != nil {
		t.Fatalf("validate startup inputs: %v", err)
	}

	t.Setenv("BLDR_TEST_ENV", "changed")
	if err := validateStartupInputs(controllerConfig, inputManifest); err == nil {
		t.Fatal("expected env validation error")
	}
}

func TestValidateStartupInputsRequiresCacheFormat(t *testing.T) {
	controllerConfig := &configset_proto.ControllerConfig{}
	controllerConfigDigest, err := marshalControllerConfigDigest(controllerConfig)
	if err != nil {
		t.Fatal(err)
	}

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewControllerConfigDigestStartupInput(controllerConfigDigest),
	)
	if err := validateStartupInputs(controllerConfig, inputManifest); err == nil {
		t.Fatal("expected missing startup cache format marker error")
	}
}

func TestValidateStartupInputsRejectsOldCacheFormat(t *testing.T) {
	controllerConfig := &configset_proto.ControllerConfig{}
	controllerConfigDigest, err := marshalControllerConfigDigest(controllerConfig)
	if err != nil {
		t.Fatal(err)
	}

	inputManifest := bldr_manifest_builder.NewInputManifest(nil, nil)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewControllerConfigDigestStartupInput(controllerConfigDigest),
	)
	inputManifest.AddStartupInput(
		bldr_manifest_builder.NewEnvStartupInput("BLDR_STARTUP_CACHE_FORMAT_V7", ""),
	)

	err = validateStartupInputs(controllerConfig, inputManifest)
	if err == nil || !strings.Contains(err.Error(), "missing startup cache format marker") {
		t.Fatalf("validate startup inputs error = %v, want missing current cache format", err)
	}
}

func TestEnrichBuilderResultForStartupReuse(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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

	if err := enrichBuilderResultForStartupReuse(builderConfig, &configset_proto.ControllerConfig{}, builderResult); err != nil {
		t.Fatal(err)
	}

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
	if startupCacheFormatEnvKey != "BLDR_STARTUP_CACHE_FORMAT_V9" {
		t.Fatalf("startup cache format marker = %s, want V9", startupCacheFormatEnvKey)
	}
}

func TestManifestDepsEqual(t *testing.T) {
	cachedDeps := []*bldr_manifest_builder.InputManifest_ManifestDep{
		{
			ManifestId:  "web",
			ManifestRef: &bucket.ObjectRef{BucketId: "bucket-a"},
		},
	}
	currentDeps := []*bldr_manifest_builder.InputManifest_ManifestDep{
		{
			ManifestId:  "web",
			ManifestRef: &bucket.ObjectRef{BucketId: "bucket-a"},
		},
	}

	if !manifestDepsEqual(cachedDeps, currentDeps) {
		t.Fatal("expected manifest deps to match")
	}
	currentDeps[0].ManifestRef = &bucket.ObjectRef{BucketId: "bucket-b"}
	if manifestDepsEqual(cachedDeps, currentDeps) {
		t.Fatal("expected manifest deps mismatch")
	}
}

func TestControllerStartupCacheHitSkipsBuild(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, true, nil)
	if buildCalls != 0 {
		t.Fatalf("expected 0 build calls, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "startup-bucket" {
		t.Fatal("expected startup builder result to be reused")
	}
}

func TestControllerPersistsAndReusesSubManifestResults(t *testing.T) {
	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "main.go")
	childPath := filepath.Join(tmpDir, "child.ts")
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("export const child = true;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	testStartupCacheBuilderState.buildSubManifest.Store(true)
	t.Cleanup(func() {
		testStartupCacheBuilderState.buildSubManifest.Store(false)
	})

	startupResult, buildCalls := runStartupExecuteTest(t, tmpDir, nil, true, nil)
	if buildCalls != 2 {
		t.Fatalf("initial build calls = %d, want parent and child", buildCalls)
	}
	if startupResult.GetSubManifestResults()["child"] == nil {
		t.Fatal("parent result did not persist child builder result")
	}
	var childStartupFile bool
	for _, inputFile := range startupResult.GetInputManifest().GetFiles() {
		if inputFile.GetPath() == "child.ts" && inputFile.GetStartupOnly() {
			childStartupFile = true
		}
	}
	if !childStartupFile {
		t.Fatal("parent result did not retain child input for startup validation")
	}

	_, buildCalls = runStartupExecuteTest(t, tmpDir, startupResult, true, nil)
	if buildCalls != 0 {
		t.Fatalf("unchanged build calls = %d, want 0", buildCalls)
	}

	if err := os.WriteFile(mainPath, []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, buildCalls = runStartupExecuteTest(t, tmpDir, startupResult, true, nil)
	if buildCalls != 1 {
		t.Fatalf("parent-only mutation build calls = %d, want parent only", buildCalls)
	}

	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("export const child = false;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, buildCalls = runStartupExecuteTest(t, tmpDir, startupResult, true, nil)
	if buildCalls != 2 {
		t.Fatalf("child mutation build calls = %d, want parent and child", buildCalls)
	}
}

func TestControllerStartupCacheHitPublishesLifecycleStatusOrdering(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	sink := newRecordingLifecycleSink()
	result, buildCalls := runStartupExecuteWithLifecycle(
		t,
		tmpDir,
		startupBuilderResult,
		true,
		false,
		nil,
		sink,
	)
	if buildCalls != 0 {
		t.Fatalf("expected 0 build calls, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "startup-bucket" {
		t.Fatal("expected startup builder result to be reused")
	}
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
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, true, nil)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerStartupFileMissPublishesFullBuildLifecycle(t *testing.T) {
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
	sink := newRecordingLifecycleSink()
	result, buildCalls := runStartupExecuteWithLifecycle(
		t,
		tmpDir,
		startupBuilderResult,
		true,
		false,
		nil,
		sink,
	)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
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
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	testStartupCacheBuilderState.cacheSafe.Store(true)
	testStartupCacheBuilderState.buildCalls.Store(0)
	tb.GetStaticResolver().AddFactory(newTestStartupCacheBuilderFactory(tb.GetBus()))

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

	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

	firstResult, err := ctrl.GetResultPromise().Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if firstResult.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected initial build result")
	}
	firstPromise, waitCh := ctrl.GetResultPromise().GetPromise()
	if firstPromise == nil {
		t.Fatal("expected first result promise")
	}
	sink.waitFor(t, ctx, func(status ManifestBuilderLifecycleStatus) bool {
		return status.Summary == "watching for changes"
	})

	writeWatchedFileUntilPromiseReplaced(t, ctx, filePath, waitCh)
	secondPromise, _ := ctrl.GetResultPromise().GetPromise()
	if secondPromise == nil {
		t.Fatal("expected second result promise")
	}
	if secondPromise == firstPromise {
		t.Fatal("expected result promise to be replaced on rebuild")
	}

	secondResult, err := secondPromise.Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if secondResult.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
	hot := sink.waitFor(t, ctx, func(status ManifestBuilderLifecycleStatus) bool {
		return status.HotRebuild && status.DependencyRebuildReason == changedFilesSummary(1)
	})
	if hot.State != ManifestBuilderLifecycleStateRunning || hot.FullRebuild || hot.CacheHit {
		t.Fatalf("unexpected hot rebuild lifecycle status: %#v", hot)
	}
	if got := testStartupCacheBuilderState.buildCalls.Load(); got != 2 {
		t.Fatalf("build calls = %d, want 2", got)
	}

	cancel()
	if execErr := waitForControllerExecuteExit(t, errCh); execErr != nil && execErr != context.Canceled {
		t.Fatalf("execute: %v", execErr)
	}
}

func TestControllerStartupEnvMissRebuilds(t *testing.T) {
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
	t.Setenv("BLDR_TEST_ENV", "new")
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, true, nil)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerStartupManifestDepMissRebuilds(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	startupBuilderResult.GetInputManifest().ManifestDeps = []*bldr_manifest_builder.InputManifest_ManifestDep{
		{
			ManifestId:  "web",
			ManifestRef: &bucket.ObjectRef{BucketId: "cached-bucket"},
		},
	}
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, true, []string{"web"})
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerCapturesManifestDepThatAppearsDuringBuild(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	var expectedRef *bucket.ObjectRef
	testStartupCacheBuilderState.buildHookMtx.Lock()
	testStartupCacheBuilderState.buildHook = func(ctx context.Context, buildCall int32) error {
		if buildCall != 1 {
			return nil
		}
		_, _, err := tb.CreateManifestWithBilly(
			ctx,
			bldr_manifest.NewManifestMeta("web", bldr_manifest.BuildType_DEV, "other/platform", 2),
			"",
			nil,
			nil,
			nil,
		)
		if err != nil {
			return err
		}
		_, manifestRef, err := tb.CreateManifestWithBilly(
			ctx,
			bldr_manifest.NewManifestMeta("web", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1),
			"",
			nil,
			nil,
			nil,
		)
		if err == nil {
			expectedRef = manifestRef.GetManifestRef()
		}
		return err
	}
	testStartupCacheBuilderState.buildHookMtx.Unlock()
	t.Cleanup(func() {
		testStartupCacheBuilderState.buildHookMtx.Lock()
		testStartupCacheBuilderState.buildHook = nil
		testStartupCacheBuilderState.buildHookMtx.Unlock()
	})

	result, buildCalls := runStartupExecuteWithTestbed(
		t,
		tb,
		tmpDir,
		nil,
		true,
		[]string{"web"},
		tb.GetWorldEngineID(),
	)
	if buildCalls != 1 {
		t.Fatalf("build calls = %d, want 1", buildCalls)
	}
	deps := result.GetInputManifest().GetManifestDeps()
	if len(deps) != 1 || deps[0].GetManifestId() != "web" ||
		!deps[0].GetManifestRef().EqualVT(expectedRef) {
		t.Fatalf("persisted manifest deps = %v, want platform-matched web ref", deps)
	}
}

func TestControllerStartupUnsafeBuilderRebuilds(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStartupBuilderResult(t, tmpDir, builderControllerConfig)
	result, buildCalls := runStartupExecuteTest(t, tmpDir, startupBuilderResult, false, nil)
	if buildCalls != 1 {
		t.Fatalf("expected 1 build call, got %d", buildCalls)
	}
	if result.GetManifestRef().GetManifestRef().GetBucketId() != "built-bucket" {
		t.Fatal("expected rebuilt result")
	}
}

func TestControllerStartupMissingManifestRebuilds(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStoredStartupBuilderResult(t, tb, tmpDir, builderControllerConfig)
	startupBuilderResult.ManifestRef.ManifestRef = startupBuilderResult.GetManifestRef().GetManifestRef().CloneVT()
	startupBuilderResult.ManifestRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff

	result, buildCalls := runStartupExecuteWithTestbed(
		t,
		tb,
		tmpDir,
		startupBuilderResult,
		true,
		nil,
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
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStoredStartupBuilderResult(t, tb, tmpDir, builderControllerConfig)
	startupBuilderResult.ManifestRef.ManifestRef = startupBuilderResult.GetManifestRef().GetManifestRef().CloneVT()
	startupBuilderResult.ManifestRef.ManifestRef.BucketId = "other-bucket"

	result, buildCalls := runStartupExecuteWithTestbed(
		t,
		tb,
		tmpDir,
		startupBuilderResult,
		true,
		nil,
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
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	ctrlRel, err := tb.GetBus().AddController(ctx, startupCacheBlockingLookupController{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRel()

	builderControllerConfig := newTestBuilderControllerProto(t)
	startupBuilderResult := buildStoredStartupBuilderResult(t, tb, tmpDir, builderControllerConfig)
	cachedBucketID := startupBuilderResult.GetManifestRef().GetManifestRef().GetBucketId()
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

	startupBuilderResult.ManifestRef.ManifestRef = startupBuilderResult.GetManifestRef().GetManifestRef().CloneVT()
	startupBuilderResult.ManifestRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff

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
	t.Helper()

	meta := bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1)
	builderResult := bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "startup-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	if err := enrichBuilderResultForStartupReuse(
		&bldr_manifest_builder.BuilderConfig{
			ManifestMeta: meta,
			SourcePath:   sourcePath,
		},
		controllerConfig,
		builderResult,
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
	t.Helper()

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

	builderResult := bldr_manifest_builder.NewBuilderResult(
		manifest,
		manifestRef.GetManifestRef(),
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
	)
	if err := enrichBuilderResultForStartupReuse(
		&bldr_manifest_builder.BuilderConfig{
			ManifestMeta: meta,
			SourcePath:   sourcePath,
			EngineId:     tb.GetWorldEngineID(),
		},
		controllerConfig,
		builderResult,
	); err != nil {
		t.Fatal(err)
	}
	return builderResult
}

func newTestBuilderControllerProto(t *testing.T) *configset_proto.ControllerConfig {
	t.Helper()

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
	watchManifestIDs []string,
) (*bldr_manifest_builder.BuilderResult, int32) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	return runStartupExecuteWithTestbed(
		t,
		tb,
		sourcePath,
		startupBuilderResult,
		cacheSafe,
		watchManifestIDs,
		"",
	)
}

func runStartupExecuteWithTestbed(
	t *testing.T,
	tb *testbed.Testbed,
	sourcePath string,
	startupBuilderResult *bldr_manifest_builder.BuilderResult,
	cacheSafe bool,
	watchManifestIDs []string,
	engineID string,
) (*bldr_manifest_builder.BuilderResult, int32) {
	t.Helper()

	testStartupCacheBuilderState.cacheSafe.Store(cacheSafe)
	testStartupCacheBuilderState.buildCalls.Store(0)
	tb.GetStaticResolver().AddFactory(newTestStartupCacheBuilderFactory(tb.GetBus()))
	tb.GetStaticResolver().AddFactory(NewFactory(tb.GetBus()))
	ctx := tb.GetContext()

	builderControllerConfig, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(1, &testStartupCacheBuilderConfig{}),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

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
	controllerConfig.WatchManifestIds = watchManifestIDs

	ctrl := NewController(tb.GetLogger(), tb.GetBus(), controllerConfig)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

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
	watchManifestIDs []string,
	sink *recordingLifecycleSink,
) (*bldr_manifest_builder.BuilderResult, int32) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rootLogger := logrus.New()
	rootLogger.SetLevel(logrus.DebugLevel)
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(rootLogger))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	testStartupCacheBuilderState.cacheSafe.Store(cacheSafe)
	testStartupCacheBuilderState.buildCalls.Store(0)
	tb.GetStaticResolver().AddFactory(newTestStartupCacheBuilderFactory(tb.GetBus()))

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
	controllerConfig.WatchManifestIds = watchManifestIDs

	ctrl := NewController(tb.GetLogger(), tb.GetBus(), controllerConfig)
	ctrl.SetManifestBuilderLifecycleSink(sink)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ctrl.Execute(ctx)
	}()

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
	t.Helper()
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
	t.Helper()

	retry := time.NewTicker(150 * time.Millisecond)
	defer retry.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

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
	t.Helper()

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	select {
	case err := <-errCh:
		return err
	case <-timeout.C:
		t.Fatal("timed out waiting for controller Execute to exit")
	}
	return nil
}
