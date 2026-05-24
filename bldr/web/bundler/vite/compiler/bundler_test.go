//go:build !js

package bldr_web_bundler_vite_compiler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	srpc "github.com/aperturerobotics/starpc/srpc"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
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

// TestViteCompilerBootstrapBuild verifies the vite compiler bootstrap can be
// bundled from embedded dist sources even though vendor/ is absent there.
func TestViteCompilerBootstrapBuild(t *testing.T) {
	ctx := context.Background()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../.."))

	distDir := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
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

	result := esbuild.Build(esbuild.BuildOptions{
		AbsWorkingDir: distDir,
		Outfile:       filepath.Join(t.TempDir(), "vite-bootstrap.mjs"),
		EntryPoints:   []string{bldr_web_bundler_vite.ResolveViteEntrypointPath(distDir)},
		Target:        esbuild.ES2022,
		Format:        esbuild.FormatESModule,
		Platform:      esbuild.PlatformNode,
		LogLevel:      esbuild.LogLevelSilent,
		TreeShaking:   esbuild.TreeShakingTrue,
		Drop:          esbuild.DropDebugger,
		Define: map[string]string{
			"BLDR_IS_NODE": "true",
		},
		Plugins: []esbuild.Plugin{
			bldr_esbuild_build.ExternalNodeModulesPlugin(),
			bldr_esbuild_build.GoVendorTsResolverPlugin(repoRoot),
		},
		External: []string{"@aptre/protobuf-es-lite", "starpc", "vite"},
		Bundle:   true,
		Write:    false,
	})
	if err := bldr_esbuild_build.BuildResultToErr(result); err != nil {
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
