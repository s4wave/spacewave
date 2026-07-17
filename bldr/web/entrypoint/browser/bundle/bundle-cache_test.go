//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	cache := newBundleCache(logrus.NewEntry(logrus.New()), f.buildDir, f.baseRoot)
	// Test binaries may not carry module build information. The production path
	// disables reuse in that case; this explicit identity tests cache mechanics.
	cache.compilerID = "test-esbuild@pinned"
	return cache
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
		verify, err := resultOutputPaths(result, f.buildDir)
		if err != nil {
			return nil, err
		}
		return &bundleBuildOutput{
			inputs: inputs,
			values: map[string]string{"filename": filename},
			verify: verify,
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
// source tree still reuses, matching the harness re-syncing its dist sources.
func TestBundleCacheContentKeyedIgnoresModtime(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")

	f.write(t, "renderer.ts", "import {shared} from './shared'\nexport const renderer = shared + 1\n")
	f.write(t, "shared.ts", "export const shared = 1\n")

	warm := f.newCache()
	if f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("renderer rebuilt after a content-identical rewrite; content keying failed")
	}
}

// TestBundleCacheFourBundleInvalidation proves one changed graph rebuilds only
// its own service, shared, OPFS, or renderer bundle kind.
func TestBundleCacheFourBundleInvalidation(t *testing.T) {
	f := newBundleCacheFixture(t)
	entries := []struct {
		name  string
		entry string
	}{
		{"service-worker", "service.ts"},
		{"shared-worker", "shared-worker.ts"},
		{"opfs-worker", "opfs.ts"},
		{"renderer", "renderer.ts"},
	}
	f.write(t, "service.ts", "export const service = 1\n")
	f.write(t, "shared-worker.ts", "export const sharedWorker = 1\n")
	f.write(t, "opfs.ts", "export const opfs = 1\n")

	cold := f.newCache()
	for _, item := range entries {
		if !f.build(t, cold, item.name, item.entry) {
			t.Fatalf("cold %s build should compile", item.name)
		}
	}
	f.write(t, "service.ts", "export const service = 2\n")

	warm := f.newCache()
	for _, item := range entries {
		built := f.build(t, warm, item.name, item.entry)
		if item.name == "service-worker" && !built {
			t.Fatal("service-worker should rebuild after its input changed")
		}
		if item.name != "service-worker" && built {
			t.Fatalf("%s rebuilt after an unrelated service-worker input change", item.name)
		}
	}
	if warm.Builds() != 1 || warm.Reuses() != 3 {
		t.Fatalf("warm seam counts = built %d reused %d, want 1 and 3", warm.Builds(), warm.Reuses())
	}
}

// TestBundleCacheOutputContentInvalidates proves a damaged output cannot be
// reused even when its size and modification time are unchanged.
func TestBundleCacheOutputContentInvalidates(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")

	outputPath := filepath.Join(f.buildDir, "renderer.js")
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	contents[0] ^= 1
	if err := os.WriteFile(outputPath, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(outputPath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	warm := f.newCache()
	if !f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("renderer should rebuild after output content corruption")
	}
}

// TestBundleCacheConfigInputsInvalidate proves tsconfig changes invalidate the
// bundle even though esbuild does not report the config in its input graph.
func TestBundleCacheConfigInputsInvalidate(t *testing.T) {
	f := newBundleCacheFixture(t)
	f.write(t, "tsconfig.json", `{"compilerOptions":{"target":"ES2022"}}`)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")

	f.write(t, "tsconfig.json", `{"compilerOptions":{"target":"ES2020"}}`)
	warm := f.newCache()
	if !f.build(t, warm, "renderer", "renderer.ts") {
		t.Fatal("renderer should rebuild after tsconfig changed")
	}
}

// TestBundleCacheCompilerIdentityRequired proves a process without a pinned
// esbuild module identity always builds rather than guessing compatibility.
func TestBundleCacheCompilerIdentityRequired(t *testing.T) {
	f := newBundleCacheFixture(t)
	cache := f.newCache()
	cache.compilerID = ""
	if !f.build(t, cache, "renderer", "renderer.ts") {
		t.Fatal("compiler-unknown build should compile")
	}
	if !f.build(t, cache, "renderer", "renderer.ts") {
		t.Fatal("compiler-unknown build should always compile")
	}
	if cache.Reuses() != 0 {
		t.Fatal("compiler-unknown build reused a cache record")
	}
}

// TestBundleCacheVirtualInputAlwaysBuilds proves an input the cache cannot map
// to a file is treated as incomplete provenance rather than cached optimistically.
func TestBundleCacheVirtualInputAlwaysBuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	opts := f.opts("renderer.ts")
	cache := f.newCache()
	build := func() {
		_, err := cache.build("virtual", opts, nil, func() (*bundleBuildOutput, error) {
			result, _, err := runEsbuildBundle(opts)
			if err != nil {
				return nil, err
			}
			verify, err := resultOutputPaths(result, f.buildDir)
			if err != nil {
				return nil, err
			}
			return &bundleBuildOutput{inputs: []string{"virtual:generated"}, verify: verify}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	build()
	build()
	if cache.Builds() != 2 || cache.Reuses() != 0 {
		t.Fatalf("virtual input seam counts = built %d reused %d, want 2 and 0", cache.Builds(), cache.Reuses())
	}
}

// TestBundleCacheConcurrentBuilds proves same-name builds serialize through the
// shared cache lock and cannot publish competing provenance records.
func TestBundleCacheConcurrentBuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	opts := f.opts("renderer.ts")
	var callbackBuilds atomic.Int32
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		cache := f.newCache()
		wg.Go(func() {
			<-start
			_, err := cache.build("renderer", opts, nil, func() (*bundleBuildOutput, error) {
				callbackBuilds.Add(1)
				result, inputs, err := runEsbuildBundle(opts)
				if err != nil {
					return nil, err
				}
				verify, err := resultOutputPaths(result, f.buildDir)
				if err != nil {
					return nil, err
				}
				return &bundleBuildOutput{inputs: inputs, verify: verify}, nil
			})
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if callbackBuilds.Load() != 1 {
		t.Fatalf("concurrent build callbacks = %d, want 1", callbackBuilds.Load())
	}
}

// TestBundleCacheConfigDigestChangeRebuilds proves a config change unrelated to
// source files invalidates reuse.
func TestBundleCacheConfigDigestChangeRebuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	f.build(t, cold, "renderer", "renderer.ts")

	warm := f.newCache()
	opts := f.opts("renderer.ts")
	opts.MinifySyntax = true
	before := warm.Builds()
	_, err := warm.build("renderer", opts, nil, func() (*bundleBuildOutput, error) {
		result, inputs, err := runEsbuildBundle(opts)
		if err != nil {
			return nil, err
		}
		verify, err := resultOutputPaths(result, f.buildDir)
		if err != nil {
			return nil, err
		}
		return &bundleBuildOutput{inputs: inputs, verify: verify}, nil
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if warm.Builds() == before {
		t.Fatal("renderer should rebuild after a config digest change")
	}
}

// TestBundleCacheExtraDigestChangeRebuilds proves a non-esbuild input change,
// such as the renderer index-map digest, invalidates reuse.
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
			verify, err := resultOutputPaths(result, f.buildDir)
			if err != nil {
				return nil, err
			}
			return &bundleBuildOutput{inputs: inputs, verify: verify}, nil
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

// TestBundleCacheMissingOutputRebuilds proves a removed output invalidates reuse.
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
