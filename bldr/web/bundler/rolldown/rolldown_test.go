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
	root := t.TempDir()
	req := validTestRequest(root)
	if err := os.MkdirAll(req.WorkingDir, 0o755); err != nil {
		t.Fatal(err)
	}
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copy := req.CloneVT()
			copy.Loaders = nil
			test.edit(copy)
			err := ValidateBuildRequest(copy)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateBuildRequest() error = %v, want substring %q", err, test.want)
			}
		})
	}
	if err := ValidateBuildRequest(req); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
}

func TestValidateBuildRequestSplittingAndIIFE(t *testing.T) {
	root := t.TempDir()
	req := validTestRequest(root)
	req.CodeSplitting = true
	req.Format = "iife"
	if err := ValidateBuildRequest(req); err == nil {
		t.Fatal("expected iife splitting to be rejected")
	}
	req.CodeSplitting = false
	req.Entrypoints = append(req.Entrypoints, &Entrypoint{Name: "other", InputPath: filepath.Join(root, "src", "other.ts")})
	if err := ValidateBuildRequest(req); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected iife entrypoint error, got %v", err)
	}
}

func TestValidateBuildResultRejectsUnnormalizedFields(t *testing.T) {
	root := t.TempDir()
	result := &BuildResult{
		Inputs:  []string{"relative.ts"},
		Outputs: []*BuildOutput{{Path: "../escape.js", Type: "javascript", Bytes: 1}},
		Tool:    &ToolIdentity{RolldownVersion: "1", BunVersion: "1", Platform: "darwin", Arch: "arm64"},
	}
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "inputs") {
		t.Fatalf("expected absolute input rejection, got %v", err)
	}
	result.Inputs = []string{filepath.Join(root, "main.ts")}
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "output-root-contained") {
		t.Fatalf("expected contained output rejection, got %v", err)
	}
	result.Outputs[0].Path = "main.js"
	result.Outputs[0].Type = "css"
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "invalid type") {
		t.Fatalf("expected output type rejection, got %v", err)
	}
	result.Outputs[0].Type = "javascript"
	result.Outputs[0].Bytes = -1
	if err := validateBuildResult(result, root); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("expected byte count rejection, got %v", err)
	}
}

func TestBuildRunnerFailureReturnsStructuredDiagnostics(t *testing.T) {
	root := t.TempDir()
	req := validTestRequest(root)
	prepareRunnerFixture(t, root, `cat > "$2" <<'JSON'
{"diagnostics":[{"severity":"error","message":"synthetic failure","code":"BLDR_TEST"}]}
JSON
exit 23`)
	fakeBun := prepareFakeBun(t, root)
	t.Setenv("PATH", filepath.Dir(fakeBun)+string(os.PathListSeparator)+os.Getenv("PATH"))
	errResult, err := Build(context.Background(), logrus.NewEntry(logrus.New()), "", req.BldrDistRoot, req)
	if err == nil || !strings.Contains(err.Error(), "synthetic failure") {
		t.Fatalf("Build() error = %v, want structured diagnostic", err)
	}
	if errResult == nil || len(errResult.GetDiagnostics()) != 1 {
		t.Fatalf("Build() result = %#v, want parsed diagnostics", errResult)
	}
}

func TestBuildCancellationUsesContextAndReapsRunner(t *testing.T) {
	root := t.TempDir()
	req := validTestRequest(root)
	prepareRunnerFixture(t, root, "sleep 30")
	fakeBun := prepareFakeBun(t, root)
	t.Setenv("PATH", filepath.Dir(fakeBun)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := Build(ctx, logrus.NewEntry(logrus.New()), "", req.BldrDistRoot, req)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Build() error = %v, want context deadline", err)
	}
}

func TestBuildConcurrentCallsUsePrivateProtocolFiles(t *testing.T) {
	root := t.TempDir()
	prepareRunnerFixture(t, root, `printf '{"inputs":["%s"],"outputs":[{"path":"main.js","type":"javascript","bytes":"1","gzip_bytes":"1"}],"tool":{"rolldown_version":"1","bun_version":"1","platform":"darwin","arch":"arm64"}}\n' "$PWD/main.ts" > "$2"`)
	bldrDistRoot := validTestRequest(root).BldrDistRoot
	fakeBun := prepareFakeBun(t, root)
	t.Setenv("PATH", filepath.Dir(fakeBun)+string(os.PathListSeparator)+os.Getenv("PATH"))
	const calls = 4
	var wg sync.WaitGroup
	errs := make(chan error, calls)
	for i := range calls {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			callRoot := filepath.Join(root, fmt.Sprintf("call-%d", i))
			req := validTestRequest(callRoot)
			req.BldrDistRoot = bldrDistRoot
			if err := os.MkdirAll(req.WorkingDir, 0o755); err != nil {
				errs <- err
				return
			}
			if _, err := Build(context.Background(), logrus.NewEntry(logrus.New()), "", req.BldrDistRoot, req); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Build() error: %v", err)
	}
}

func TestEnsureDependencyRootRejectsStaleSourceRolldown(t *testing.T) {
	root := t.TempDir()
	depsRoot := filepath.Join(root, "bldr", "dist", "deps")
	sourcePackage := []byte(`{"dependencies":{"rolldown":"1.2.3"}}`)
	for path, data := range map[string][]byte{
		filepath.Join(depsRoot, "package.json"):                                                         sourcePackage,
		filepath.Join(depsRoot, "node_modules", "rolldown", "package.json"):                             []byte(`{"version":"1.2.2"}`),
		filepath.Join(depsRoot, "node_modules", "rolldown", "dist", "index.mjs"):                        []byte("stale"),
		filepath.Join(root, "state", "build-web-pkgs", "node_modules", "rolldown", "dist", "index.mjs"): []byte("current"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	packageHash := sha256.Sum256(sourcePackage)
	installRoot := filepath.Join(root, "state", "build-web-pkgs")
	if err := os.WriteFile(filepath.Join(installRoot, ".bldr-install-hash"), fmt.Appendf(nil, "%x", packageHash), 0o644); err != nil {
		t.Fatal(err)
	}

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
	t.Helper()
	req := validTestRequest(root)
	for _, dir := range []string{req.WorkingDir, req.SourceRoot, req.OutputRoot, filepath.Join(req.BldrDistRoot, "dist", "deps", "node_modules", "rolldown", "dist"), filepath.Dir(filepath.Join(req.BldrDistRoot, "web", "bundler", "rolldown", "run-build.mjs"))} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(req.BldrDistRoot, "dist", "deps", "node_modules", "rolldown", "dist", "index.mjs"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(req.BldrDistRoot, "dist", "deps", "package.json"), []byte(`{"dependencies":{"rolldown":"1.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(req.BldrDistRoot, "dist", "deps", "node_modules", "rolldown", "package.json"), []byte(`{"version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join(req.BldrDistRoot, "web", "bundler", "rolldown", "run-build.mjs")
	if err := os.WriteFile(runner, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
