package bldr_web_bundler_rolldown

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func validTestRequest(root string) *BuildRequest {
	return &BuildRequest{
		WorkingDir:     filepath.Join(root, "work"),
		SourceRoot:     filepath.Join(root, "src"),
		OutputRoot:     filepath.Join(root, "out"),
		Entrypoints:    []*Entrypoint{{Name: "main", InputPath: filepath.Join(root, "src", "main.ts")}},
		Format:         "es",
		Platform:       "browser",
		EntryFileNames: "entry/[name].js",
		ChunkFileNames: "chunk/[name]-[hash].js",
		AssetFileNames: "asset/[name][extname]",
		Sourcemap:      "none",
		BldrDistRoot:   filepath.Join(root, "bldr"),
		TreeShaking:    true,
	}
}

func TestValidateBuildRequestContract(t *testing.T) {
	// Set up a valid request rooted in a temporary directory.
	root := t.TempDir()
	req := validTestRequest(root)
	if err := os.MkdirAll(req.WorkingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Define invalid request variants and their expected validation errors.
	tests := []struct {
		name string
		edit func(*BuildRequest)
		want string
	}{
		{"relative working directory", func(r *BuildRequest) { r.WorkingDir = "work" }, "working_dir"},
		{"relative source root", func(r *BuildRequest) { r.SourceRoot = "src" }, "source_root"},
		{"relative output root", func(r *BuildRequest) { r.OutputRoot = "out" }, "output_root"},
		{"relative bldr root", func(r *BuildRequest) { r.BldrDistRoot = "bldr" }, "bldr_dist_root"},
		{"missing entrypoint name", func(r *BuildRequest) { r.Entrypoints[0].Name = "" }, "name"},
		{"relative entrypoint", func(r *BuildRequest) { r.Entrypoints[0].InputPath = "main.ts" }, "input_path"},
		{"duplicate entrypoint names", func(r *BuildRequest) {
			r.Entrypoints = append(r.Entrypoints, &Entrypoint{Name: "main", InputPath: filepath.Join(root, "src", "other.ts")})
		}, "duplicated"},
		{"invalid format", func(r *BuildRequest) { r.Format = "umd" }, "format"},
		{"missing iife global name", func(r *BuildRequest) { r.Format = "iife" }, "global_name"},
		{"invalid platform", func(r *BuildRequest) { r.Platform = "deno" }, "platform"},
		{"invalid sourcemap", func(r *BuildRequest) { r.Sourcemap = "true" }, "sourcemap"},
		{"splitting cjs", func(r *BuildRequest) { r.CodeSplitting = true; r.Format = "cjs" }, "code_splitting"},
		{"missing entry naming", func(r *BuildRequest) { r.EntryFileNames = "" }, "entry_file_names"},
		{"invalid loader", func(r *BuildRequest) { r.Loaders = map[string]string{".css": "css"} }, "loader"},
	}

	// Run each invalid request case in its own subtest.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Clone the valid request and apply the selected invalid field.
			copy := req.CloneVT()
			copy.Loaders = nil
			test.edit(copy)
			err := ValidateBuildRequest(copy)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateBuildRequest() error = %v, want substring %q", err, test.want)
			}
		})
	}

	// Confirm the unmodified request passes validation.
	if err := ValidateBuildRequest(req); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestValidateBuildRequestSplittingAndIIFE(t *testing.T) {
	// Prepare an IIFE request with code splitting enabled.
	root := t.TempDir()
	req := validTestRequest(root)
	req.CodeSplitting = true
	req.Format = "iife"
	if err := ValidateBuildRequest(req); err == nil {
		t.Fatal("expected iife splitting to be rejected")
	}

	// Disable splitting and add a second entrypoint for the next rejection.
	req.CodeSplitting = false
	req.Entrypoints = append(req.Entrypoints, &Entrypoint{Name: "other", InputPath: filepath.Join(root, "src", "other.ts")})
	if err := ValidateBuildRequest(req); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected iife entrypoint error, got %v", err)
	}
}

func TestValidateBuildResultRejectsUnnormalizedFields(t *testing.T) {
	// Create a result with an invalid input, escaped output, and tool metadata.
	root := t.TempDir()
	result := &BuildResult{
		Inputs:  []string{"relative.ts"},
		Outputs: []*BuildOutput{{Path: "../escape.js", Type: "javascript", Bytes: 1}},
		Tool:    &ToolIdentity{RolldownVersion: "1", BunVersion: "1", Platform: "darwin", Arch: "arm64"},
	}
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "inputs") {
		t.Fatalf("expected absolute input rejection, got %v", err)
	}

	// Make the input absolute and verify the escaped output path is rejected.
	result.Inputs = []string{filepath.Join(root, "main.ts")}
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "output-root-contained") {
		t.Fatalf("expected contained output rejection, got %v", err)
	}

	// Normalize the output path and reject an unsupported output type.
	result.Outputs[0].Path = "main.js"
	result.Outputs[0].Type = "css"
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected output type rejection, got %v", err)
	}

	// Restore the JavaScript output type and reject a negative byte count.
	result.Outputs[0].Type = "javascript"
	result.Outputs[0].Bytes = -1
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("expected byte count rejection, got %v", err)
	}
}

func TestBuildRunnerFailureReturnsStructuredDiagnostics(t *testing.T) {
	// Prepare a fake runner that emits a structured build error.
	root := t.TempDir()
	req := validTestRequest(root)
	prepareRunnerFixture(t, root, `cat > "$2" <<'JSON'
{"diagnostics":[{"severity":"error","message":"synthetic failure","code":"BLDR_TEST"}]}
JSON
exit 23`)
	fakeBun := prepareFakeBun(t, root)
	t.Setenv("PATH", filepath.Dir(fakeBun)+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Run the build and verify its structured failure.
	errResult, err := Build(context.Background(), logrus.NewEntry(logrus.New()), "", req.BldrDistRoot, req)
	if err == nil || !strings.Contains(err.Error(), "synthetic failure") {
		t.Fatalf("Build() error = %v, want structured diagnostic", err)
	}

	// Check that the failed build still returns parsed diagnostics.
	if errResult == nil || len(errResult.GetDiagnostics()) != 1 {
		t.Fatalf("Build() result = %#v, want parsed diagnostics", errResult)
	}
}

func TestBuildCancellationUsesContextAndReapsRunner(t *testing.T) {
	// Prepare a fake bundler process that waits for cancellation.
	root := t.TempDir()
	req := validTestRequest(root)
	prepareRunnerFixture(t, root, "sleep 30")
	fakeBun := prepareFakeBun(t, root)
	t.Setenv("PATH", filepath.Dir(fakeBun)+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Give the build a short deadline to trigger process cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run the build under the deadline and require context cancellation.
	_, err := Build(ctx, logrus.NewEntry(logrus.New()), "", req.BldrDistRoot, req)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Build() error = %v, want context deadline", err)
	}
}

func TestBuildConcurrentCallsUsePrivateProtocolFiles(t *testing.T) {
	// Prepare a runner fixture for concurrent isolated builds.
	root := t.TempDir()
	prepareRunnerFixture(t, root, `printf '{"inputs":["%s"],"outputs":[{"path":"main.js","type":"javascript","bytes":"1"}],"tool":{"rolldown_version":"1","bun_version":"1","platform":"darwin","arch":"arm64"}}\n' "$PWD/main.ts" > "$2"`)
	bldrDistRoot := validTestRequest(root).BldrDistRoot
	fakeBun := prepareFakeBun(t, root)
	t.Setenv("PATH", filepath.Dir(fakeBun)+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Start several builds against the shared protocol fixture.
	const calls = 4
	var wg sync.WaitGroup
	errs := make(chan error, calls)
	for i := range calls {
		wg.Add(1)
		go func(i int) {
			// Release the worker count when each build goroutine exits.
			defer wg.Done()

			// Give each worker a private directory and build request.
			callRoot := filepath.Join(root, fmt.Sprintf("call-%d", i))
			req := validTestRequest(callRoot)
			req.BldrDistRoot = bldrDistRoot

			// Create the worker directory before starting its build.
			if err := os.MkdirAll(req.WorkingDir, 0o755); err != nil {
				errs <- err
				return
			}

			// Build the worker request and report any failure to the result channel.
			if _, err := Build(context.Background(), logrus.NewEntry(logrus.New()), "", req.BldrDistRoot, req); err != nil {
				errs <- err
			}
		}(i)
	}

	// Wait for every concurrent build to finish.
	wg.Wait()

	// Close the error channel after all workers have returned.
	close(errs)

	// Report each build error collected from the workers.
	for err := range errs {
		t.Errorf("concurrent Build() error: %v", err)
	}
}

func TestEnsureDependencyRootRejectsStaleSourceRolldown(t *testing.T) {
	// Prepare a stale vendored Rolldown package and a current shared install.
	root := t.TempDir()
	depsRoot := filepath.Join(root, "bldr", "dist", "deps")
	sourcePackage := []byte(`{"dependencies":{"rolldown":"1.2.3"}}`)

	// The shared install cache holds a current install keyed by the manifest.
	packageHash := fmt.Sprintf("%x", sha256.Sum256(sourcePackage))
	cacheRoot := filepath.Join(root, "cache")
	t.Setenv("BLDR_SHARED_INSTALL_CACHE", cacheRoot)
	installRoot := filepath.Join(cacheRoot, packageHash)
	for path, data := range map[string][]byte{
		filepath.Join(depsRoot, "package.json"):                                     sourcePackage,
		filepath.Join(depsRoot, "node_modules", "rolldown", "package.json"):         []byte(`{"version":"1.2.2"}`),
		filepath.Join(depsRoot, "node_modules", "rolldown", "dist", "index.mjs"):    []byte("stale"),
		filepath.Join(installRoot, "node_modules", "rolldown", "dist", "index.mjs"): []byte("current"),
		filepath.Join(installRoot, ".bldr-install-hash"):                            []byte(packageHash),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Resolve dependencies and require the current shared install to replace stale source.
	got, err := ensureDependencyRoot(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		filepath.Join(root, "state"),
		filepath.Join(root, "bldr"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got != installRoot {
		t.Fatalf("ensureDependencyRoot() = %q, want managed root %q", got, installRoot)
	}
}

func prepareFakeBun(t *testing.T, root string) string {
	// Create a fake Bun executable that runs the supplied script.
	t.Helper()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "bun")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func prepareRunnerFixture(t *testing.T, root, body string) {
	// Prepare the directories and dependency files required by the runner.
	t.Helper()
	req := validTestRequest(root)
	for _, dir := range []string{req.WorkingDir, req.SourceRoot, req.OutputRoot, filepath.Join(req.BldrDistRoot, "dist", "deps", "node_modules", "rolldown", "dist"), filepath.Dir(filepath.Join(req.BldrDistRoot, "web", "bundler", "rolldown", "run-build.mjs"))} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Write the dependency metadata and Rolldown fixture files.
	if err := os.WriteFile(filepath.Join(req.BldrDistRoot, "dist", "deps", "node_modules", "rolldown", "dist", "index.mjs"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(req.BldrDistRoot, "dist", "deps", "package.json"), []byte(`{"dependencies":{"rolldown":"1.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(req.BldrDistRoot, "dist", "deps", "node_modules", "rolldown", "package.json"), []byte(`{"version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write the fake build runner script.
	runner := filepath.Join(req.BldrDistRoot, "web", "bundler", "rolldown", "run-build.mjs")
	if err := os.WriteFile(runner, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
