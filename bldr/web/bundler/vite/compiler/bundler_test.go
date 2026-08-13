//go:build !js

package bldr_web_bundler_vite_compiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	srpc "github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_web_bundler "github.com/s4wave/spacewave/bldr/web/bundler"
	bldr_web_bundler_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/sirupsen/logrus"
)

type fakeViteBundlerClient struct {
	buildRequest *bldr_web_bundler_vite.BuildRequest
}

func (f *fakeViteBundlerClient) SRPCClient() srpc.Client { return nil }

func (f *fakeViteBundlerClient) Build(_ context.Context, req *bldr_web_bundler_vite.BuildRequest) (*bldr_web_bundler_vite.BuildResponse, error) {
	f.buildRequest = req
	return &bldr_web_bundler_vite.BuildResponse{Success: true}, nil
}

func (f *fakeViteBundlerClient) BuildWebPkg(context.Context, *bldr_web_bundler_vite.BuildWebPkgRequest) (*bldr_web_bundler_vite.BuildWebPkgResponse, error) {
	return nil, nil
}

// TestViteCompilerBootstrapBuild verifies the vite compiler bootstrap resolves
// vendored @go imports from the generated dist source tree, not the app root.
func TestViteCompilerBootstrapBuild(t *testing.T) {
	ctx := context.Background()

	distDir := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(sourceRoot, "go.mod"),
		[]byte("module github.com/example/app\n\ngo 1.26\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	le := logrus.NewEntry(logrus.New())
	distSourcesHandle := bldr.BuildDistSourcesFSHandle(ctx, le)
	defer distSourcesHandle.Release()

	err := unixfs_sync.Sync(
		ctx,
		distDir,
		distSourcesHandle,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		unixfs_sync.NewSkipPathPrefixes([]string{"vendor", "node_modules"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	pipesockDir := filepath.Join(distDir, "vendor", "github.com", "aperturerobotics", "util", "pipesock")
	if err := os.MkdirAll(pipesockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(pipesockDir, "pipesock.ts"),
		[]byte(`export function buildPipeName() { return "" }
export function createSocketConnection() { return null }
export function startSocketSender() { return undefined }`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "vite-bootstrap.mjs")
	if _, _, err := bldr_web_bundler_vite.BuildServiceScript(
		ctx,
		le,
		t.TempDir(),
		sourceRoot,
		distDir,
		outputPath,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatal(err)
	}
}

func TestResolveViteBaseConfigPathMonorepo(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "bldr/web/bundler/vite/vite-base.config.ts")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := bldr_web_bundler_vite.ResolveViteBaseConfigPath(tmpDir)
	if got != "bldr/web/bundler/vite/vite-base.config.ts" {
		t.Fatalf("unexpected vite base config path: %q", got)
	}
}

func TestBuildViteBundleMetaMergesDuplicateBundleMetadata(t *testing.T) {
	got, err := BuildViteBundleMeta([]*ViteBundleMeta{
		{
			Id: "frontend",
			Entrypoints: []*ViteBundleEntrypoint{
				{InputPath: "src/app.ts"},
			},
			ViteConfigPaths: []string{"vite.project.config.ts"},
			ExternalPkgs:    []string{"@spacewave/project-runtime"},
		},
		{
			Id: "frontend",
			Entrypoints: []*ViteBundleEntrypoint{
				{InputPath: "src/worker.ts"},
			},
			ViteConfigPaths:      []string{"vite.compiler.config.ts"},
			ExternalPkgs:         []string{"react", "@aptre/bldr"},
			DisableProjectConfig: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("merged bundle count=%d want 1", len(got))
	}

	bundle := got[0]
	if bundle.GetId() != "frontend" {
		t.Fatalf("merged bundle id=%q want frontend", bundle.GetId())
	}
	entrypoints := bundle.GetEntrypoints()
	if len(entrypoints) != 2 {
		t.Fatalf("merged entrypoint count=%d want 2", len(entrypoints))
	}
	if entrypoints[0].GetInputPath() != "src/app.ts" || entrypoints[1].GetInputPath() != "src/worker.ts" {
		t.Fatalf("merged entrypoints=%v want [src/app.ts src/worker.ts]", []string{
			entrypoints[0].GetInputPath(),
			entrypoints[1].GetInputPath(),
		})
	}
	if got, want := bundle.GetViteConfigPaths(), []string{"vite.project.config.ts", "vite.compiler.config.ts"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("merged vite config paths=%v want %v", got, want)
	}
	if got, want := bundle.GetExternalPkgs(), []string{"@spacewave/project-runtime", "react", "@aptre/bldr"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("merged external packages=%v want %v", got, want)
	}
	if !bundle.GetDisableProjectConfig() {
		t.Fatal("merged bundle did not preserve DisableProjectConfig")
	}
}

func TestBuildViteBundlePropagatesJavaScriptPolicy(t *testing.T) {
	codeRoot := t.TempDir()
	distRoot := t.TempDir()
	outAssets := t.TempDir()
	workingPath := t.TempDir()
	client := &fakeViteBundlerClient{}

	_, _, _, err := BuildViteBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		distRoot,
		codeRoot,
		workingPath,
		nil,
		&ViteBundleMeta{
			Id: "default",
			Entrypoints: []*ViteBundleEntrypoint{{
				InputPath: "src/main.ts",
			}},
			DisableProjectConfig: true,
		},
		client,
		nil,
		outAssets,
		"plugin-id",
		true,
		false,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.buildRequest == nil {
		t.Fatal("missing vite build request")
	}
	if client.buildRequest.GetMode() != "production" {
		t.Fatalf("request mode=%q want production", client.buildRequest.GetMode())
	}
	if client.buildRequest.GetJsMinification() {
		t.Fatal("request enabled JavaScript minification")
	}
	if !client.buildRequest.GetJsSourcemaps() {
		t.Fatal("request did not enable JavaScript sourcemaps")
	}
}

func TestBuildViteBundleOmitsExcludedWebPackagesFromBuildRequest(t *testing.T) {
	codeRoot := t.TempDir()
	distRoot := t.TempDir()
	outAssets := t.TempDir()
	workingPath := t.TempDir()
	client := &fakeViteBundlerClient{}

	_, _, _, err := BuildViteBundle(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		distRoot,
		codeRoot,
		workingPath,
		nil,
		&ViteBundleMeta{
			Id: "default",
			Entrypoints: []*ViteBundleEntrypoint{{
				InputPath: "src/main.ts",
			}},
			DisableProjectConfig: true,
		},
		client,
		[]*bldr_web_bundler.WebPkgRefConfig{
			{Id: "sonner", Exclude: true, Imports: []string{"toast"}},
			{Id: "@spacewave/ui", Imports: []string{"Button"}},
			{Id: "react", Imports: []string{"jsx"}},
		},
		outAssets,
		"plugin-id",
		false,
		true,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if client.buildRequest == nil {
		t.Fatal("missing vite build request")
	}

	got := client.buildRequest.GetWebPkgs()
	if len(got) != 2 {
		t.Fatalf("request web package count=%d want 2: %v", len(got), got)
	}
	gotImportsByID := make(map[string][]string, len(got))
	for _, ref := range got {
		if ref.GetExclude() {
			t.Fatalf("request included excluded web package ref: %s", ref.GetId())
		}
		gotImportsByID[ref.GetId()] = ref.GetImports()
	}
	if _, ok := gotImportsByID["sonner"]; ok {
		t.Fatal("request included excluded web package sonner")
	}
	if gotImports := gotImportsByID["@spacewave/ui"]; len(gotImports) != 1 || gotImports[0] != "Button" {
		t.Fatalf("request imports for @spacewave/ui=%v want [Button]", gotImports)
	}
	if gotImports := gotImportsByID["react"]; len(gotImports) != 1 || gotImports[0] != "jsx" {
		t.Fatalf("request imports for react=%v want [jsx]", gotImports)
	}
}

func TestBuildInputManifestDeduplicatesSharedSources(t *testing.T) {
	sourcePath := t.TempDir()
	sharedPath := filepath.Join(sourcePath, "app", "shared.ts")
	viteOnlyPath := filepath.Join(sourcePath, "app", "vite.ts")

	inputManifest, err := (&Controller{}).buildInputManifest(
		sourcePath,
		&viteBuildResult{viteSrcFiles: []string{
			sharedPath,
			"app/shared.ts",
			viteOnlyPath,
		}},
		nil,
		nil,
		[]string{sharedPath},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := inputManifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(inputManifest.GetFiles()) != 2 {
		t.Fatalf("input file count=%d want 2", len(inputManifest.GetFiles()))
	}

	kinds := make(map[string]InputFileKind, len(inputManifest.GetFiles()))
	for _, inputFile := range inputManifest.GetFiles() {
		meta := &InputFileMeta{}
		if err := meta.UnmarshalVT(inputFile.GetMetadata()); err != nil {
			t.Fatal(err)
		}
		kinds[filepath.ToSlash(inputFile.GetPath())] = meta.GetKind()
	}
	if got := kinds["app/shared.ts"]; got != InputFileKind_InputFileKind_WEB_PKG {
		t.Fatalf("shared source kind=%v want WEB_PKG", got)
	}
	if got := kinds["app/vite.ts"]; got != InputFileKind_InputFileKind_VITE {
		t.Fatalf("vite source kind=%v want VITE", got)
	}
}

func TestStopViteBundlersRemovesReleaseKeys(t *testing.T) {
	controller := &Controller{}
	controller.viteBundlers = keyed.NewKeyedRefCount(
		func(viteBundlerKey) (keyed.Routine, *viteBundlerTracker) {
			return nil, &viteBundlerTracker{}
		},
	)

	key := newViteBundlerKey("/dist", "/src", "/work", "fe")
	ref, _, _ := controller.viteBundlers.AddKeyRef(key)

	controller.stopViteBundlers("/dist", "/src", "/work", []*ViteBundleMeta{{
		Id: "fe",
	}})
	ref.Release()

	if got := len(controller.viteBundlers.GetKeys()); got != 0 {
		t.Fatalf("vite bundler keys after release cleanup=%d want 0", got)
	}
}
