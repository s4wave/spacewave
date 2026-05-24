package bldr_manifest_builder_controller

import (
	"context"
	"os"
	"path/filepath"
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
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
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
	cacheSafe  atomic.Bool
	buildCalls atomic.Int32
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
	testStartupCacheBuilderState.buildCalls.Add(1)
	builderConfig := args.GetBuilderConfig()
	meta := builderConfig.GetManifestMeta().CloneVT()
	return bldr_manifest_builder.NewBuilderResult(
		bldr_manifest.NewManifest(meta, "dist/demo"),
		&bucket.ObjectRef{BucketId: "built-bucket"},
		bldr_manifest_builder.NewInputManifest([]string{"main.go"}, nil),
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
	if startupCacheFormatEnvKey != "BLDR_STARTUP_CACHE_FORMAT_V4" {
		t.Fatalf("startup cache format marker = %s, want V4", startupCacheFormatEnvKey)
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
	ctx := tb.GetContext()

	builderControllerConfig, err := configset_proto.NewControllerConfig(
		configset.NewControllerConfig(1, &testStartupCacheBuilderConfig{}),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	builderConfig := &bldr_manifest_builder.BuilderConfig{
		ManifestMeta: bldr_manifest.NewManifestMeta("demo", bldr_manifest.BuildType_DEV, "desktop/linux/amd64", 1),
		SourcePath:   sourcePath,
		EngineId:     engineID,
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
