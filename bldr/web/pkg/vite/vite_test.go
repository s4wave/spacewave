//go:build !js

package web_pkg_vite

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	srpc "github.com/aperturerobotics/starpc/srpc"
	bldr_vite "github.com/s4wave/spacewave/bldr/web/bundler/vite"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	"github.com/sirupsen/logrus"
)

type fakeViteBundlerClient struct {
	resp      *bldr_vite.BuildWebPkgResponse
	buildResp func(*bldr_vite.BuildWebPkgRequest) *bldr_vite.BuildWebPkgResponse
	requests  []*bldr_vite.BuildWebPkgRequest
}

func (f *fakeViteBundlerClient) SRPCClient() srpc.Client { return nil }

func (f *fakeViteBundlerClient) Build(context.Context, *bldr_vite.BuildRequest) (*bldr_vite.BuildResponse, error) {
	return nil, nil
}

func (f *fakeViteBundlerClient) BuildWebPkg(_ context.Context, req *bldr_vite.BuildWebPkgRequest) (*bldr_vite.BuildWebPkgResponse, error) {
	f.requests = append(f.requests, req)
	if f.buildResp != nil {
		return f.buildResp(req), nil
	}
	return f.resp, nil
}

func TestBuildWebPkgsViteKeepsRelativeSourceFiles(t *testing.T) {
	codeRootPath := t.TempDir()
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "@aptre", "it-ws")
	outDir := filepath.Join(t.TempDir(), "out")
	for _, source := range []string{
		filepath.Join(pkgRoot, "dist/src/duplex.js"),
		filepath.Join(pkgRoot, "dist/src/socket.js"),
	} {
		if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("export {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	client := &fakeViteBundlerClient{
		resp: &bldr_vite.BuildWebPkgResponse{
			Success: true,
			SourceFiles: []string{
				"node_modules/@aptre/it-ws/dist/src/duplex.js",
				filepath.Join(pkgRoot, "dist/src/socket.js"),
			},
		},
	}

	_, srcFiles, _, err := BuildWebPkgsViteWithManagedRoot(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		codeRootPath,
		filepath.Join(codeRootPath, ".state"),
		[]*web_pkg.WebPkgRef{{
			WebPkgId:   "@aptre/it-ws",
			WebPkgRoot: pkgRoot,
		}},
		outDir,
		"/b/pkg/",
		false,
		false,
		true,
		client,
		filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}

	slices.Sort(srcFiles)
	expected := []string{
		"node_modules/@aptre/it-ws/dist/src/duplex.js",
		"node_modules/@aptre/it-ws/dist/src/socket.js",
	}
	if !slices.Equal(srcFiles, expected) {
		t.Fatalf("unexpected source files: got %v want %v", srcFiles, expected)
	}
}

func TestBuildWebPkgsViteUsesOneAbsoluteWrapperIdentity(t *testing.T) {
	codeRootPath := t.TempDir()
	workingDir := t.TempDir()
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	pkgRoot := filepath.Join(codeRootPath, "node_modules", "stable-cjs")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "index.cjs"), []byte("exports.stable = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join("relative-output", "plugin-C2-b7nSr")
	var wrapperPath string
	client := &fakeViteBundlerClient{buildResp: func(req *bldr_vite.BuildWebPkgRequest) *bldr_vite.BuildWebPkgResponse {
		wrapperPath = req.GetImports()[0]
		return &bldr_vite.BuildWebPkgResponse{Success: true, SourceFiles: []string{wrapperPath}}
	}}
	_, sourceFiles, _, err := BuildWebPkgsViteWithManagedRoot(
		context.Background(), logrus.NewEntry(logrus.New()), codeRootPath, filepath.Join(codeRootPath, ".state"),
		[]*web_pkg.WebPkgRef{{WebPkgId: "stable-cjs", WebPkgRoot: pkgRoot, Imports: []string{"index.cjs"}}},
		outputPath, "/b/pkg/", true, false, true, client, filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(wrapperPath) {
		t.Fatalf("wrapper path = %q, want absolute", wrapperPath)
	}
	if _, err := os.Stat(wrapperPath); err != nil {
		t.Fatalf("generated wrapper does not exist: %v", err)
	}
	if want := []string{"node_modules/stable-cjs/index.cjs"}; !slices.Equal(sourceFiles, want) {
		t.Fatalf("source files = %v, want %v", sourceFiles, want)
	}
	if err := os.RemoveAll(filepath.Join(workingDir, "relative-output")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wrapperPath); !os.IsNotExist(err) {
		t.Fatalf("generated wrapper survived cleanup: %v", err)
	}
}

func TestBuildWebPkgsViteIgnoresGeneratedOutputSources(t *testing.T) {
	codeRootPath := t.TempDir()
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(codeRootPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "stable-pkg")
	stableSource := filepath.Join(pkgRoot, "index.cjs")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stableSource, []byte("exports.stable = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	build := func(outputName string, outputThroughSymlink, reportThroughSymlink bool) []string {
		realOutput := filepath.Join(codeRootPath, ".bldr", "build", outputName)
		aliasOutput := filepath.Join(codeRootPath, ".bldr", "aliases", outputName)
		if err := os.MkdirAll(filepath.Dir(aliasOutput), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(realOutput, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realOutput, aliasOutput); err != nil {
			t.Fatal(err)
		}
		outputPath := realOutput
		if outputThroughSymlink {
			outputPath = aliasOutput
		}
		relativeOutput := filepath.Join(".bldr", "build", outputName)
		if outputThroughSymlink {
			relativeOutput = filepath.Join(".bldr", "aliases", outputName)
		}
		var generatedPath string
		client := &fakeViteBundlerClient{
			buildResp: func(req *bldr_vite.BuildWebPkgRequest) *bldr_vite.BuildWebPkgResponse {
				generatedPath = req.GetImports()[0]
				generatedAbs, absErr := filepath.Abs(generatedPath)
				if absErr != nil {
					t.Fatal(absErr)
				}
				reportedGenerated := generatedAbs
				if reportThroughSymlink != outputThroughSymlink {
					rel, relErr := filepath.Rel(outputPath, generatedAbs)
					if relErr != nil {
						t.Fatal(relErr)
					}
					if reportThroughSymlink {
						reportedGenerated = filepath.Join(aliasOutput, rel)
					} else {
						reportedGenerated = filepath.Join(realOutput, rel)
					}
				}
				return &bldr_vite.BuildWebPkgResponse{
					Success:     true,
					SourceFiles: []string{stableSource, reportedGenerated},
				}
			},
		}
		_, sourceFiles, _, err := BuildWebPkgsViteWithManagedRoot(
			context.Background(),
			logrus.NewEntry(logrus.New()),
			codeRootPath,
			filepath.Join(codeRootPath, ".state"),
			[]*web_pkg.WebPkgRef{{
				WebPkgId:   "stable-pkg",
				WebPkgRoot: pkgRoot,
				Imports:    []string{"index.cjs"},
			}},
			relativeOutput,
			"/b/pkg/",
			false,
			false,
			true,
			client,
			filepath.Join(t.TempDir(), "cache"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(generatedPath); err != nil {
			t.Fatalf("generated wrapper was not created: %v", err)
		}
		if err := os.RemoveAll(realOutput); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(generatedPath); !os.IsNotExist(err) {
			t.Fatalf("generated wrapper survived output cleanup: %v", err)
		}
		return sourceFiles
	}

	first := build("plugin-C2-b7nSr", true, false)
	second := build("plugin-B9-x4kLm", false, true)
	want := []string{"node_modules/stable-pkg/index.cjs"}
	if !slices.Equal(first, want) || !slices.Equal(second, want) {
		t.Fatalf("startup sources changed with generated output: first=%v second=%v want=%v", first, second, want)
	}
}

func TestBuildWebPkgsViteLegacyCall(t *testing.T) {
	client := &fakeViteBundlerClient{resp: &bldr_vite.BuildWebPkgResponse{Success: true}}
	_, _, _, err := BuildWebPkgsVite(
		context.Background(), logrus.NewEntry(logrus.New()), t.TempDir(), nil,
		t.TempDir(), "/b/pkg/", false, false, true, client, t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildWebPkgsViteIgnoresManagedStateSources(t *testing.T) {
	const managedSuffix = "build/js/spacewave-app/sub/vite/build/web-pkgs/node_modules/@aptre/protobuf-es-lite/dist/assert.js"

	run := func(t *testing.T, codeRootPath, managedRootPath, managedSource string) {
		pkgRoot := filepath.Join(codeRootPath, "node_modules", "stable-pkg")
		stableSource := filepath.Join(pkgRoot, "index.js")
		for _, source := range []string{stableSource, managedSource} {
			if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte("export {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		client := &fakeViteBundlerClient{resp: &bldr_vite.BuildWebPkgResponse{
			Success:     true,
			SourceFiles: []string{stableSource, managedSource},
		}}
		_, sourceFiles, _, err := BuildWebPkgsViteWithManagedRoot(
			context.Background(), logrus.NewEntry(logrus.New()), codeRootPath, managedRootPath,
			[]*web_pkg.WebPkgRef{{WebPkgId: "stable-pkg", WebPkgRoot: pkgRoot}},
			filepath.Join(t.TempDir(), "assets"),
			"/b/pkg/", true, false, true, client, filepath.Join(t.TempDir(), "cache"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"node_modules/stable-pkg/index.js"}; !slices.Equal(sourceFiles, want) {
			t.Fatalf("source files = %v, want %v", sourceFiles, want)
		}
	}

	t.Run("default", func(t *testing.T) {
		codeRootPath := t.TempDir()
		managedRootPath := filepath.Join(codeRootPath, ".bldr")
		run(t, codeRootPath, managedRootPath, filepath.Join(managedRootPath, managedSuffix))
	})

	t.Run("custom-relative", func(t *testing.T) {
		codeRootPath := t.TempDir()
		oldWorkingDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(codeRootPath); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chdir(oldWorkingDir); err != nil {
				t.Errorf("restore working directory: %v", err)
			}
		})
		run(t, codeRootPath, ".state", filepath.Join(codeRootPath, ".state", managedSuffix))
	})

	t.Run("absolute", func(t *testing.T) {
		codeRootPath := t.TempDir()
		managedRootPath := t.TempDir()
		run(t, codeRootPath, managedRootPath, filepath.Join(managedRootPath, managedSuffix))
	})
}

func TestBuildWebPkgsViteTracksConditionalReexportSources(t *testing.T) {
	codeRootPath := t.TempDir()
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "conditional-pkg")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "index.js"), []byte(`
if (process.env.NODE_ENV === "production") {
  module.exports = require("bare-target")
} else {
  module.exports = require("./dev.js")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	bareTargetRoot := filepath.Join(codeRootPath, "node_modules", "bare-target")
	if err := os.MkdirAll(bareTargetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareTargetRoot, "package.json"), []byte(`{"main":"index.js"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareTargetRoot, "index.js"), []byte("exports.production = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "dev.js"), []byte("exports.development = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	client := &fakeViteBundlerClient{buildResp: func(req *bldr_vite.BuildWebPkgRequest) *bldr_vite.BuildWebPkgResponse {
		return &bldr_vite.BuildWebPkgResponse{Success: true, SourceFiles: req.GetImports()}
	}}
	_, sourceFiles, _, err := BuildWebPkgsViteWithManagedRoot(
		context.Background(), logrus.NewEntry(logrus.New()), codeRootPath, filepath.Join(codeRootPath, ".state"),
		[]*web_pkg.WebPkgRef{{WebPkgId: "conditional-pkg", WebPkgRoot: pkgRoot, Imports: []string{"index.js"}}},
		filepath.Join(codeRootPath, ".bldr", "output"), "/b/pkg/", true, false, true,
		client, filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(sourceFiles)
	want := []string{
		"node_modules/bare-target/index.js",
		"node_modules/bare-target/package.json",
		"node_modules/conditional-pkg/index.js",
	}
	if !slices.Equal(sourceFiles, want) {
		t.Fatalf("conditional wrapper sources = %v, want %v", sourceFiles, want)
	}
}

func TestBuildWebPkgsViteKeepsCjsWrappersOutsideOutDir(t *testing.T) {
	codeRootPath := t.TempDir()
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "cjs-pkg")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(pkgRoot, "index.cjs"),
		[]byte("exports.alpha = 1;\nexports.beta = 2;\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	client := &fakeViteBundlerClient{
		resp: &bldr_vite.BuildWebPkgResponse{Success: true},
	}

	_, _, _, err := BuildWebPkgsViteWithManagedRoot(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		codeRootPath,
		filepath.Join(codeRootPath, ".state"),
		[]*web_pkg.WebPkgRef{{
			WebPkgId:   "cjs-pkg",
			WebPkgRoot: pkgRoot,
			Imports:    []string{"index.cjs"},
		}},
		outDir,
		"/b/pkg/",
		false,
		false,
		true,
		client,
		filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("unexpected request count: got %d want 1", len(client.requests))
	}

	req := client.requests[0]
	if len(req.GetImports()) != 1 {
		t.Fatalf("unexpected imports: %v", req.GetImports())
	}
	wrapperPath := req.GetImports()[0]
	if !filepath.IsAbs(wrapperPath) {
		t.Fatalf("wrapper path is not absolute: %s", wrapperPath)
	}
	outPrefix := req.GetOutDir() + string(os.PathSeparator)
	if strings.HasPrefix(wrapperPath, outPrefix) {
		t.Fatalf("wrapper path %s is inside outDir %s", wrapperPath, req.GetOutDir())
	}
	expectedPrefix, err := filepath.EvalSymlinks(filepath.Join(outDir, ".cjs-wrappers"))
	if err != nil {
		t.Fatal(err)
	}
	expectedPrefix += string(os.PathSeparator)
	if !strings.HasPrefix(wrapperPath, expectedPrefix) {
		t.Fatalf("wrapper path %s does not use wrapper dir prefix %s", wrapperPath, expectedPrefix)
	}
	if strings.Contains(filepath.ToSlash(wrapperPath), "/cjs-pkg/index.mjs") {
		t.Fatalf("wrapper path %s includes package id in entrypoint name", wrapperPath)
	}
}

func TestBuildWebPkgsVitePropagatesJavaScriptPolicy(t *testing.T) {
	codeRootPath := t.TempDir()
	pkgRoot := filepath.Join(codeRootPath, "node_modules", "policy-pkg")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	client := &fakeViteBundlerClient{
		resp: &bldr_vite.BuildWebPkgResponse{Success: true},
	}

	_, _, _, err := BuildWebPkgsViteWithManagedRoot(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		codeRootPath,
		filepath.Join(codeRootPath, ".state"),
		[]*web_pkg.WebPkgRef{{
			WebPkgId:   "policy-pkg",
			WebPkgRoot: pkgRoot,
			Imports:    []string{"index.js"},
		}},
		filepath.Join(t.TempDir(), "out"),
		"/b/pkg/",
		true,
		false,
		true,
		client,
		filepath.Join(t.TempDir(), "cache"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("unexpected request count: got %d want 1", len(client.requests))
	}
	req := client.requests[0]
	if !req.GetIsRelease() {
		t.Fatal("request did not preserve release mode")
	}
	if req.GetJsMinification() {
		t.Fatal("request enabled JavaScript minification")
	}
	if !req.GetJsSourcemaps() {
		t.Fatal("request did not enable JavaScript sourcemaps")
	}
}
