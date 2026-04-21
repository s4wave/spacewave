//go:build !js

package bldr_web_bundler_vite_compiler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	bldr "github.com/s4wave/spacewave/bldr"
	bldr_esbuild_build "github.com/s4wave/spacewave/bldr/web/bundler/esbuild/build"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/sirupsen/logrus"
)

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
		EntryPoints:   []string{"./web/bundler/vite/vite.ts"},
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
