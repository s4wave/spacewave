//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"
	"testing"

	esbuild "github.com/aperturerobotics/esbuild/pkg/api"
	"github.com/sirupsen/logrus"
)

// bundleCacheFixture is a temporary source tree plus build dir used to exercise
// the bundle cache with real esbuild builds.
type bundleCacheFixture struct {
	baseRoot string
	buildDir string
}

func newBundleCacheFixture(t *testing.T) *bundleCacheFixture {
	t.Helper()
	root := t.TempDir()
	f := &bundleCacheFixture{
		baseRoot: filepath.Join(root, "src"),
		buildDir: filepath.Join(root, "out"),
	}
	// renderer bundle graph: renderer.ts imports shared.ts.
	f.write(t, "renderer.ts", "import {shared} from './shared'\nexport const renderer = shared + 1\n")
	f.write(t, "shared.ts", "export const shared = 1\n")
	// worker bundle graph: worker.ts imports worker-impl.ts.
	f.write(t, "worker.ts", "import {run} from './worker-impl'\nexport const worker = run()\n")
	f.write(t, "worker-impl.ts", "export function run() { return 2 }\n")
	return f
}

func (f *bundleCacheFixture) write(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(f.baseRoot, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func (f *bundleCacheFixture) newCache() *bundleCache {
	return newBundleCache(logrus.NewEntry(logrus.New()), f.buildDir, f.baseRoot)
}

func (f *bundleCacheFixture) opts(entryRel string) esbuild.BuildOptions {
	return esbuild.BuildOptions{
		AbsWorkingDir: f.baseRoot,
		EntryPoints:   []string{entryRel},
		Outdir:        f.buildDir,
		Bundle:        true,
		Write:         true,
		Format:        esbuild.FormatESModule,
		Platform:      esbuild.PlatformBrowser,
		Target:        esbuild.ES2022,
	}
}

// build runs a single-output bundle through the cache and reports whether it was
// compiled (a cache miss) rather than reused.
func (f *bundleCacheFixture) build(t *testing.T, cache *bundleCache, name, entryRel string) (built bool) {
	t.Helper()
	before := cache.Builds()
	opts := f.opts(entryRel)
	_, err := cache.build(name, opts, nil, func() (*bundleBuildOutput, error) {
		result, inputs, err := runEsbuildBundle(opts)
		if err != nil {
			return nil, err
		}
		filename, err := singleWorkerOutputName(result)
		if err != nil {
			return nil, err
		}
		return &bundleBuildOutput{
			inputs: inputs,
			values: map[string]string{"filename": filename},
			verify: []string{filename},
		}, nil
	})
	if err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	return cache.Builds() > before
}

// TestBundleCacheWarmReuseZeroBuilds proves a second build of an unchanged tree
// performs zero bundle builds, asserted through the cache build counter.
func TestBundleCacheWarmReuseZeroBuilds(t *testing.T) {
	f := newBundleCacheFixture(t)

	cold := f.newCache()
	if !f.build(t, cold, "renderer", "renderer.ts") {
		t.Fatal("cold renderer build should compile")
	}
	if !f.build(t, cold, "worker", "worker.ts") {
		t.Fatal("cold worker build should compile")
	}
	if cold.Builds() != 2 {
		t.Fatalf("cold builds = %d, want 2", cold.Builds())
	}

	warm := f.newCache()
	if f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("warm renderer build should reuse, not compile")
	}
	if f.build(t, warm, "worker", "worker.ts") {
		t.Fatal("warm worker build should reuse, not compile")
	}
	if warm.Builds() != 0 {
		t.Fatalf("warm builds = %d, want 0", warm.Builds())
	}
	if warm.Reuses() != 2 {
		t.Fatalf("warm reuses = %d, want 2", warm.Reuses())
	}
}

// TestBundleCacheContentKeyedIgnoresModtime proves a rewritten-but-identical
// source tree (fresh modification times, unchanged bytes) still reuses, matching
// the harness re-syncing its dist sources on every boot.
func TestBundleCacheContentKeyedIgnoresModtime(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")

	// Rewrite both inputs with identical content, bumping their modtimes.
	f.write(t, "renderer.ts", "import {shared} from './shared'\nexport const renderer = shared + 1\n")
	f.write(t, "shared.ts", "export const shared = 1\n")

	warm := f.newCache()
	if f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("renderer rebuilt after a content-identical rewrite; content keying failed")
	}
}

// TestBundleCacheEntrypointSourceInvalidatesOnlyRenderer proves touching a
// renderer-graph source rebuilds only the renderer bundle.
func TestBundleCacheEntrypointSourceInvalidatesOnlyRenderer(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")
	f.build(t, cold, "worker", "worker.ts")

	f.write(t, "shared.ts", "export const shared = 42\n")

	warm := f.newCache()
	if !f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("renderer should rebuild after its input changed")
	}
	if f.build(t, warm, "worker", "worker.ts") {
		t.Fatal("worker should reuse when only a renderer input changed")
	}
}

// TestBundleCacheWorkerSourceInvalidatesOnlyWorker proves touching a worker-graph
// source rebuilds only that worker bundle.
func TestBundleCacheWorkerSourceInvalidatesOnlyWorker(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")
	f.build(t, cold, "worker", "worker.ts")

	f.write(t, "worker-impl.ts", "export function run() { return 99 }\n")

	warm := f.newCache()
	if !f.build(t, warm, "worker", "worker.ts") {
		t.Fatal("worker should rebuild after its input changed")
	}
	if f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("renderer should reuse when only a worker input changed")
	}
}

// TestBundleCacheConfigDigestChangeRebuilds proves a config change (unrelated to
// source files) invalidates reuse.
func TestBundleCacheConfigDigestChangeRebuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")

	warm := f.newCache()
	opts := f.opts("renderer.ts")
	opts.MinifySyntax = true // change the esbuild option digest
	before := warm.Builds()
	_, err := warm.build("renderer", opts, nil, func() (*bundleBuildOutput, error) {
		result, inputs, err := runEsbuildBundle(opts)
		if err != nil {
			return nil, err
		}
		filename, err := singleWorkerOutputName(result)
		if err != nil {
			return nil, err
		}
		return &bundleBuildOutput{inputs: inputs, values: map[string]string{"filename": filename}, verify: []string{filename}}, nil
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if warm.Builds() == before {
		t.Fatal("renderer should rebuild after a config digest change")
	}
}

// TestBundleCacheExtraDigestChangeRebuilds proves a non-esbuild input change
// (for example the renderer index-map digest) invalidates reuse.
func TestBundleCacheExtraDigestChangeRebuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	opts := f.opts("renderer.ts")
	buildOnce := func(cache *bundleCache, extra []byte) bool {
		before := cache.Builds()
		_, err := cache.build("renderer", opts, extra, func() (*bundleBuildOutput, error) {
			result, inputs, err := runEsbuildBundle(opts)
			if err != nil {
				return nil, err
			}
			filename, err := singleWorkerOutputName(result)
			if err != nil {
				return nil, err
			}
			return &bundleBuildOutput{inputs: inputs, values: map[string]string{"filename": filename}, verify: []string{filename}}, nil
		})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return cache.Builds() > before
	}

	if !buildOnce(f.newCache(), []byte("index-v1")) {
		t.Fatal("cold build should compile")
	}
	if buildOnce(f.newCache(), []byte("index-v1")) {
		t.Fatal("same extra digest should reuse")
	}
	if !buildOnce(f.newCache(), []byte("index-v2")) {
		t.Fatal("changed extra digest should rebuild")
	}
}

// TestBundleCacheMissingOutputRebuilds proves a removed output file invalidates
// reuse even when the inputs are unchanged.
func TestBundleCacheMissingOutputRebuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")

	if err := os.Remove(filepath.Join(f.buildDir, "renderer.js")); err != nil {
		t.Fatalf("remove output: %v", err)
	}

	warm := f.newCache()
	if !f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("renderer should rebuild after its output was removed")
	}
}
