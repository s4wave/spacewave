//go:build !js

package entrypoint_browser_bundle

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
)

type bundleCacheFixture struct {
	baseRoot string
	buildDir string
}

func newBundleCacheFixture(t *testing.T) *bundleCacheFixture {
	// Create renderer and worker sources for the bundle cache.
	t.Helper()
	root := t.TempDir()
	f := &bundleCacheFixture{
		baseRoot: filepath.Join(root, "src"),
		buildDir: filepath.Join(root, "out"),
	}
	f.write(t, "renderer.ts", "export const renderer = 1\n")
	f.write(t, "worker.ts", "export const worker = 2\n")
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

func (f *bundleCacheFixture) spec(entryRel, request string) bundleCacheSpec {
	if request == "" {
		request = entryRel
	}
	return bundleCacheSpec{
		compilerID:  "rolldown-test@pinned",
		request:     []byte(request),
		configFiles: []string{filepath.Join(f.baseRoot, "tsconfig.json")},
	}
}

func (f *bundleCacheFixture) build(t *testing.T, cache *bundleCache, name, entryRel string) bool {
	t.Helper()
	return f.buildWith(t, cache, name, entryRel, f.spec(entryRel, ""), nil, nil)
}

func (f *bundleCacheFixture) buildWith(
	t *testing.T,
	cache *bundleCache,
	name,
	entryRel string,
	spec bundleCacheSpec,
	extra []byte,
	callbackCount *atomic.Int32,
) bool {
	// Build the requested bundle and track whether compilation occurred.
	t.Helper()
	before := cache.Builds()
	_, err := cache.build(name, spec, extra, func() (*bundleBuildOutput, error) {
		// Count the compiler callback and read its entrypoint source.
		if callbackCount != nil {
			callbackCount.Add(1)
		}
		contents, err := os.ReadFile(filepath.Join(f.baseRoot, entryRel))
		if err != nil {
			return nil, err
		}

		// Write the compiled entrypoint into the bundle output directory.
		if err := os.MkdirAll(f.buildDir, 0o755); err != nil {
			return nil, err
		}
		outputName := name + ".mjs"
		if err := os.WriteFile(filepath.Join(f.buildDir, outputName), contents, 0o644); err != nil {
			return nil, err
		}
		return &bundleBuildOutput{
			inputs: []string{entryRel},
			values: map[string]string{"filename": outputName},
			verify: []string{outputName},
		}, nil
	})
	if err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	return cache.Builds() > before
}

func TestBundleCacheWarmReuseZeroBuilds(t *testing.T) {
	// Create a cold cache and compile both entrypoints.
	f := newBundleCacheFixture(t)
	cold := f.newCache()
	if !f.build(t, cold, "renderer", "renderer.ts") || !f.build(t, cold, "worker", "worker.ts") {
		t.Fatal("cold bundles should compile")
	}

	// Reopen the cache and require reuse of both bundles.
	warm := f.newCache()
	if f.build(t, warm, "renderer", "renderer.ts") || f.build(t, warm, "worker", "worker.ts") {
		t.Fatal("unchanged bundles should reuse")
	}
	if warm.Builds() != 0 || warm.Reuses() != 2 {
		t.Fatalf("warm seam counts = built %d reused %d, want 0 and 2", warm.Builds(), warm.Reuses())
	}
}

func TestBundleCacheContentKeyedIgnoresModtime(t *testing.T) {
	f := newBundleCacheFixture(t)
	f.build(t, f.newCache(), "renderer", "renderer.ts")
	f.write(t, "renderer.ts", "export const renderer = 1\n")
	if f.build(t, f.newCache(), "renderer", "renderer.ts") {
		t.Fatal("content-identical source rewrite should reuse")
	}
}

func TestBundleCacheFourBundleInvalidation(t *testing.T) {
	// Create the four entrypoints tracked by the bundle cache.
	f := newBundleCacheFixture(t)
	entries := []struct{ name, entry string }{
		{"service-worker", "service.ts"},
		{"shared-worker", "shared-worker.ts"},
		{"opfs-worker", "opfs.ts"},
		{"renderer", "renderer.ts"},
	}
	for _, item := range entries[:3] {
		f.write(t, item.entry, "export const value = 1\n")
	}

	// Compile each entrypoint into the cold cache.
	cold := f.newCache()
	for _, item := range entries {
		f.build(t, cold, item.name, item.entry)
	}

	// Change the service worker and check that only its bundle rebuilds.
	f.write(t, "service.ts", "export const value = 2\n")
	warm := f.newCache()
	for _, item := range entries {
		// Build this entrypoint and verify its invalidation decision.
		built := f.build(t, warm, item.name, item.entry)
		if built != (item.name == "service-worker") {
			t.Fatalf("%s built=%t after service-worker change", item.name, built)
		}
	}

	// Verify the total build and reuse counts after invalidation.
	if warm.Builds() != 1 || warm.Reuses() != 3 {
		t.Fatalf("warm seam counts = built %d reused %d, want 1 and 3", warm.Builds(), warm.Reuses())
	}
}

func TestBundleCacheRebuildRemovesRecordedOutputs(t *testing.T) {
	// Create a renderer cache and a compiler that writes named outputs.
	f := newBundleCacheFixture(t)
	spec := f.spec("renderer.ts", "")
	build := func(outputName string) {
		// Build the renderer with the requested output name.
		_, err := f.newCache().build("renderer", spec, nil, func() (*bundleBuildOutput, error) {
			// Write the named renderer output for cache verification.
			if err := os.MkdirAll(f.buildDir, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(f.buildDir, outputName), []byte(outputName), 0o644); err != nil {
				return nil, err
			}
			return &bundleBuildOutput{
				inputs: []string{"renderer.ts"},
				verify: []string{outputName},
			}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Rebuild the changed renderer under a new output name.
	build("renderer-old.mjs")
	f.write(t, "renderer.ts", "export const renderer = 2\n")
	build("renderer-new.mjs")

	// Verify that the cache removed the old output and retained the new one.
	if _, err := os.Stat(filepath.Join(f.buildDir, "renderer-old.mjs")); !os.IsNotExist(err) {
		t.Fatalf("stale cache output survived rebuild: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.buildDir, "renderer-new.mjs")); err != nil {
		t.Fatal(err)
	}
}

func TestBundleCacheOutputContentInvalidates(t *testing.T) {
	// Build the renderer and read its recorded output metadata.
	f := newBundleCacheFixture(t)
	f.build(t, f.newCache(), "renderer", "renderer.ts")
	outputPath := filepath.Join(f.buildDir, "renderer.mjs")
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	// Read and corrupt the renderer output while preserving its modification time.
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

	// Require recompilation of the corrupted renderer output.
	if !f.build(t, f.newCache(), "renderer", "renderer.ts") {
		t.Fatal("corrupt output should rebuild")
	}
}

func TestBundleCacheConfigInputsInvalidate(t *testing.T) {
	// Compile the renderer with its initial TypeScript configuration.
	f := newBundleCacheFixture(t)
	f.write(t, "tsconfig.json", `{"compilerOptions":{"target":"ES2022"}}`)
	f.build(t, f.newCache(), "renderer", "renderer.ts")

	// Change the TypeScript configuration and require recompilation.
	f.write(t, "tsconfig.json", `{"compilerOptions":{"target":"ES2020"}}`)
	if !f.build(t, f.newCache(), "renderer", "renderer.ts") {
		t.Fatal("changed config file should rebuild")
	}
}

func TestBundleCacheViteServiceInputsInvalidate(t *testing.T) {
	// Create the Vite service inputs used by the renderer cache.
	f := newBundleCacheFixture(t)
	bundlerProtoPath := filepath.Join("bldr", "web", "bundler", "bundler.pb.ts")
	resolverPath := filepath.Join("bldr", "web", "bundler", "vite", "go-ts-resolver.ts")
	f.write(t, bundlerProtoPath, "export const bundle = 1\n")
	f.write(t, resolverPath, "export const resolver = 1\n")

	// Compile the renderer with the Vite compiler configuration.
	spec := f.spec("renderer.ts", "")
	spec.compilerID = rendererBrowserCompilerID
	spec.configFiles = browserBundleConfigFiles(f.baseRoot, spec.compilerID)
	if !f.buildWith(t, f.newCache(), "renderer", "renderer.ts", spec, nil, nil) {
		t.Fatal("cold renderer should compile")
	}

	// Change the resolver input and require recompilation.
	f.write(t, resolverPath, "export const resolver = 2\n")
	if !f.buildWith(t, f.newCache(), "renderer", "renderer.ts", spec, nil, nil) {
		t.Fatal("changed Vite service input should rebuild")
	}

	// Change the shared bundler protocol input and require recompilation.
	f.write(t, bundlerProtoPath, "export const bundle = 2\n")
	if !f.buildWith(t, f.newCache(), "renderer", "renderer.ts", spec, nil, nil) {
		t.Fatal("changed shared bundler RPC input should rebuild")
	}
}

func TestBundleCacheCompilerIdentityRequired(t *testing.T) {
	// Create a bundle cache whose compiler identity is unknown.
	f := newBundleCacheFixture(t)
	cache := f.newCache()
	spec := f.spec("renderer.ts", "")
	spec.compilerID = ""

	// Verify that requests without a compiler identity always compile.
	if !f.buildWith(t, cache, "renderer", "renderer.ts", spec, nil, nil) {
		t.Fatal("unknown compiler should compile on first request")
	}
	if !f.buildWith(t, cache, "renderer", "renderer.ts", spec, nil, nil) {
		t.Fatal("unknown compiler should compile on repeated request")
	}
	if cache.Reuses() != 0 {
		t.Fatal("unknown compiler reused a cache record")
	}
}

func TestBundleCacheVirtualInputAlwaysBuilds(t *testing.T) {
	// Create a cache and compiler for a virtual entrypoint.
	f := newBundleCacheFixture(t)
	cache := f.newCache()
	spec := f.spec("renderer.ts", "virtual")
	build := func() {
		// Build the virtual entrypoint through the cache.
		_, err := cache.build("virtual", spec, nil, func() (*bundleBuildOutput, error) {
			// Write the virtual compiler output into the build directory.
			if err := os.MkdirAll(f.buildDir, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(f.buildDir, "virtual.mjs"), []byte("x"), 0o644); err != nil {
				return nil, err
			}
			return &bundleBuildOutput{inputs: []string{"virtual:generated"}, verify: []string{"virtual.mjs"}}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Build the virtual entrypoint twice and require no cache reuse.
	build()
	build()
	if cache.Builds() != 2 || cache.Reuses() != 0 {
		t.Fatalf("virtual seam counts = built %d reused %d, want 2 and 0", cache.Builds(), cache.Reuses())
	}
}

func TestBundleCacheConcurrentBuilds(t *testing.T) {
	// Create a shared renderer fixture and synchronized build counters.
	f := newBundleCacheFixture(t)
	var callbackBuilds atomic.Int32
	start := make(chan struct{})

	// Start two cache instances that compile the same renderer concurrently.
	errs := make(chan bool, 2)
	var wg sync.WaitGroup
	for range 2 {
		cache := f.newCache()
		wg.Go(func() {
			// Wait for the common start signal and build the renderer.
			<-start
			errs <- f.buildWith(t, cache, "renderer", "renderer.ts", f.spec("renderer.ts", ""), nil, &callbackBuilds)
		})
	}

	// Release both builds and collect their completion results.
	close(start)
	wg.Wait()
	close(errs)
	for range errs {
	}

	// Verify that the cache compiled the renderer only once.
	if callbackBuilds.Load() != 1 {
		t.Fatalf("concurrent callbacks = %d, want 1", callbackBuilds.Load())
	}
}

func TestBundleCacheConfigDigestChangeRebuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	f.build(t, f.newCache(), "renderer", "renderer.ts")
	if !f.buildWith(t, f.newCache(), "renderer", "renderer.ts", f.spec("renderer.ts", "minify=true"), nil, nil) {
		t.Fatal("changed request identity should rebuild")
	}
}

func TestBundleCacheExtraDigestChangeRebuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	build := func(extra string) bool {
		return f.buildWith(t, f.newCache(), "renderer", "renderer.ts", f.spec("renderer.ts", ""), []byte(extra), nil)
	}
	if !build("index-v1") || build("index-v1") || !build("index-v2") {
		t.Fatal("extra digest did not control reuse")
	}
}

func TestBundleCacheMissingOutputRebuilds(t *testing.T) {
	f := newBundleCacheFixture(t)
	f.build(t, f.newCache(), "renderer", "renderer.ts")
	if err := os.Remove(filepath.Join(f.buildDir, "renderer.mjs")); err != nil {
		t.Fatal(err)
	}
	if !f.build(t, f.newCache(), "renderer", "renderer.ts") {
		t.Fatal("missing output should rebuild")
	}
}

func TestBundleCacheRejectsEsbuildEraFormat(t *testing.T) {
	// Compile the renderer and read its cache record.
	f := newBundleCacheFixture(t)
	f.build(t, f.newCache(), "renderer", "renderer.ts")
	recordPath := filepath.Join(f.buildDir, bundleCacheDirName, "renderer.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the renderer cache record before changing its format version.
	record, err := parseBundleRecord(data)
	if err != nil {
		t.Fatal(err)
	}

	// Store the obsolete cache format and require recompilation.
	record.formatVersion = 2
	if err := os.WriteFile(recordPath, record.marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
	if !f.build(t, f.newCache(), "renderer", "renderer.ts") {
		t.Fatal("esbuild-era cache record should rebuild")
	}
}
